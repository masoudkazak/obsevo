// Command benchmark drives synthetic ingestion load against a running server
// and reports what it costs to serve.
//
// It measures the three things that decide whether this deployment fits on a
// small VPS: how long an ingestion request takes, how far the async worker
// falls behind, and how much CPU, memory and disk the result consumes.
//
//	go run ./cmd/benchmark \
//	  -host http://localhost:3001 \
//	  -public-key pk-lf-... -secret-key sk-lf-... \
//	  -rate 1000 -duration 60s
//
// Rate is traces per minute. Each trace carries -observations spans and
// generations, so the event rate is roughly rate * (1 + observations).
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func main() {
	var (
		host           = flag.String("host", "http://localhost:3001", "base URL of the API server")
		publicKey      = flag.String("public-key", os.Getenv("LANGFUSE_PUBLIC_KEY"), "project public key")
		secretKey      = flag.String("secret-key", os.Getenv("LANGFUSE_SECRET_KEY"), "project secret key")
		rate           = flag.Int("rate", 1000, "traces per minute")
		duration       = flag.Duration("duration", 60*time.Second, "how long to sustain the load")
		observations   = flag.Int("observations", 4, "observations per trace")
		batchSize      = flag.Int("batch", 20, "events per ingestion request")
		concurrency    = flag.Int("concurrency", 8, "concurrent ingestion requests")
		databaseURL    = flag.String("database-url", os.Getenv("DATABASE_URL"), "optional: sample Postgres size")
		redisURL       = flag.String("redis-url", os.Getenv("REDIS_URL"), "optional: sample queue depth and Redis memory")
		statsContainer = flag.String("stats-container", "", "optional: docker container name to sample CPU and memory from")
		settleTimeout  = flag.Duration("settle-timeout", 2*time.Minute, "how long to wait for the worker to drain")
		outputJSON     = flag.Bool("json", false, "emit the report as JSON")
	)
	flag.Parse()

	if *publicKey == "" || *secretKey == "" {
		log.Fatal("a public key and secret key are required (see -public-key / -secret-key)")
	}
	if *rate <= 0 {
		log.Fatal("-rate must be positive")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	runner := &runner{
		host:         strings.TrimRight(*host, "/"),
		credentials:  base64.StdEncoding.EncodeToString([]byte(*publicKey + ":" + *secretKey)),
		rate:         *rate,
		duration:     *duration,
		observations: *observations,
		batchSize:    *batchSize,
		concurrency:  *concurrency,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConnsPerHost: *concurrency * 2,
			},
		},
	}

	sampler, err := newSampler(ctx, *databaseURL, *redisURL, *statsContainer)
	if err != nil {
		log.Printf("resource sampling disabled: %v", err)
	}
	defer sampler.close()

	report := runner.run(ctx, sampler)

	// The client-side latency only tells half the story: ingestion is async, so
	// the run is not really finished until the worker has drained the queue.
	report.WorkerDrainSeconds = sampler.waitForDrain(ctx, *settleTimeout)
	report.After = sampler.snapshot(ctx)

	if *outputJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			log.Fatalf("encoding report: %v", err)
		}
		return
	}
	report.print()
}

// runner drives the load.
type runner struct {
	host         string
	credentials  string
	rate         int
	duration     time.Duration
	observations int
	batchSize    int
	concurrency  int
	client       *http.Client
}

// Report is the benchmark outcome.
type Report struct {
	Scenario             string  `json:"scenario"`
	TracesPerMinute      int     `json:"traces_per_minute"`
	ObservationsPerTrace int     `json:"observations_per_trace"`
	DurationSeconds      float64 `json:"duration_seconds"`

	TracesSent   int64 `json:"traces_sent"`
	EventsSent   int64 `json:"events_sent"`
	RequestsSent int64 `json:"requests_sent"`
	Failures     int64 `json:"failures"`

	AchievedTracesPerMinute float64 `json:"achieved_traces_per_minute"`
	AchievedEventsPerSecond float64 `json:"achieved_events_per_second"`

	IngestP50Ms float64 `json:"ingest_p50_ms"`
	IngestP95Ms float64 `json:"ingest_p95_ms"`
	IngestP99Ms float64 `json:"ingest_p99_ms"`
	IngestMaxMs float64 `json:"ingest_max_ms"`

	ReadP50Ms float64 `json:"read_p50_ms"`
	ReadP95Ms float64 `json:"read_p95_ms"`

	WorkerDrainSeconds float64 `json:"worker_drain_seconds"`
	PeakQueueDepth     int64   `json:"peak_queue_depth"`

	Before Snapshot `json:"before"`
	After  Snapshot `json:"after"`

	PeakCPUPercent float64 `json:"peak_cpu_percent,omitempty"`
	PeakMemoryMB   float64 `json:"peak_memory_mb,omitempty"`
}

// Snapshot captures storage state at a point in time.
type Snapshot struct {
	PostgresBytes    int64 `json:"postgres_bytes"`
	TraceCount       int64 `json:"trace_count"`
	ObservationCount int64 `json:"observation_count"`
	RedisMemoryBytes int64 `json:"redis_memory_bytes"`
	QueueDepth       int64 `json:"queue_depth"`
}

func (r *runner) run(ctx context.Context, sampler *sampler) Report {
	report := Report{
		Scenario:             fmt.Sprintf("%d traces/min", r.rate),
		TracesPerMinute:      r.rate,
		ObservationsPerTrace: r.observations,
		Before:               sampler.snapshot(ctx),
	}

	// Each trace produces one trace event plus its observations.
	eventsPerTrace := 1 + r.observations
	totalTraces := int(float64(r.rate) * r.duration.Minutes())
	if totalTraces < 1 {
		totalTraces = 1
	}

	log.Printf("sending %d traces (%d events) over %s at %d traces/min",
		totalTraces, totalTraces*eventsPerTrace, r.duration, r.rate)

	var (
		mu        sync.Mutex
		latencies []float64
		failures  int64
		requests  int64
	)

	// Traces are emitted on a fixed schedule so the measured rate is the
	// requested one, not simply as fast as the machine can go.
	interval := r.duration / time.Duration(totalTraces)
	work := make(chan []event, r.concurrency*2)

	var workers sync.WaitGroup
	for i := 0; i < r.concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for batch := range work {
				start := time.Now()
				err := r.send(ctx, batch)
				elapsed := float64(time.Since(start).Microseconds()) / 1000

				mu.Lock()
				latencies = append(latencies, elapsed)
				requests++
				if err != nil {
					failures++
				}
				mu.Unlock()
			}
		}()
	}

	stopSampling := sampler.startSampling(ctx)

	started := time.Now()
	ticker := time.NewTicker(maxDuration(interval, time.Microsecond))
	defer ticker.Stop()

	batch := make([]event, 0, r.batchSize)
	sent := 0

emit:
	for sent < totalTraces {
		select {
		case <-ctx.Done():
			break emit
		case <-ticker.C:
		}

		batch = append(batch, newTraceEvents(r.observations)...)
		sent++

		if len(batch) >= r.batchSize {
			work <- batch
			batch = make([]event, 0, r.batchSize)
		}
	}
	if len(batch) > 0 {
		work <- batch
	}

	close(work)
	workers.Wait()
	elapsed := time.Since(started)
	peakCPU, peakMemory, peakQueue := stopSampling()

	sort.Float64s(latencies)
	report.DurationSeconds = elapsed.Seconds()
	report.TracesSent = int64(sent)
	report.EventsSent = int64(sent * eventsPerTrace)
	report.RequestsSent = requests
	report.Failures = failures
	report.AchievedTracesPerMinute = float64(sent) / elapsed.Minutes()
	report.AchievedEventsPerSecond = float64(sent*eventsPerTrace) / elapsed.Seconds()
	report.IngestP50Ms = percentile(latencies, 0.50)
	report.IngestP95Ms = percentile(latencies, 0.95)
	report.IngestP99Ms = percentile(latencies, 0.99)
	report.IngestMaxMs = percentile(latencies, 1.0)
	report.PeakCPUPercent = peakCPU
	report.PeakMemoryMB = peakMemory
	report.PeakQueueDepth = peakQueue

	report.ReadP50Ms, report.ReadP95Ms = r.measureReads(ctx)
	return report
}

// measureReads times the dashboard's own list query, which is the read path a
// user actually waits on.
func (r *runner) measureReads(ctx context.Context) (float64, float64) {
	const samples = 20

	latencies := make([]float64, 0, samples)
	for i := 0; i < samples; i++ {
		if ctx.Err() != nil {
			break
		}
		start := time.Now()
		if err := r.get(ctx, "/api/public/traces?limit=50"); err != nil {
			continue
		}
		latencies = append(latencies, float64(time.Since(start).Microseconds())/1000)
	}

	sort.Float64s(latencies)
	return percentile(latencies, 0.50), percentile(latencies, 0.95)
}

// event is one Langfuse ingestion event.
type event struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Timestamp string                 `json:"timestamp"`
	Body      map[string]interface{} `json:"body"`
}

// newTraceEvents builds one trace and its observations, shaped like the output
// of a real RAG application so the row sizes are representative.
func newTraceEvents(observations int) []event {
	traceID := randomID()
	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339Nano)

	events := []event{{
		ID:        randomID(),
		Type:      "trace-create",
		Timestamp: stamp,
		Body: map[string]interface{}{
			"id":        traceID,
			"name":      "benchmark-qa",
			"userId":    fmt.Sprintf("user-%d", rand.Intn(1000)),
			"sessionId": fmt.Sprintf("session-%d", rand.Intn(200)),
			"timestamp": stamp,
			"input":     "What does this benchmark measure and why does it matter?",
			"tags":      []string{"benchmark", "rag"},
			"release":   "v1.0.0",
		},
	}}

	parentID := ""
	for i := 0; i < observations; i++ {
		id := randomID()
		start := now.Add(time.Duration(i*50) * time.Millisecond)
		end := start.Add(120 * time.Millisecond)

		body := map[string]interface{}{
			"id":        id,
			"traceId":   traceID,
			"name":      fmt.Sprintf("step-%d", i),
			"startTime": start.Format(time.RFC3339Nano),
			"endTime":   end.Format(time.RFC3339Nano),
			"input":     "retrieved context for the question under evaluation",
			"output":    "an answer assembled from the retrieved context",
		}
		if parentID != "" {
			body["parentObservationId"] = parentID
		}

		eventType := "span-create"
		// Every other observation is a generation, so token and cost handling
		// is exercised at a realistic ratio.
		if i%2 == 1 {
			eventType = "generation-create"
			body["model"] = "gpt-4o-mini"
			body["modelParameters"] = map[string]interface{}{"temperature": 0.2}
			body["usage"] = map[string]interface{}{
				"input":  800 + rand.Intn(400),
				"output": 100 + rand.Intn(200),
			}
		}

		events = append(events, event{
			ID:        randomID(),
			Type:      eventType,
			Timestamp: stamp,
			Body:      body,
		})
		if i == 0 {
			parentID = id
		}
	}

	return events
}

func (r *runner) send(ctx context.Context, batch []event) error {
	payload, err := json.Marshal(map[string]interface{}{"batch": batch})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.host+"/api/public/ingestion", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Basic "+r.credentials)

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = resp.Body.Read(make([]byte, 0))

	if resp.StatusCode >= 300 {
		return fmt.Errorf("ingestion returned %d", resp.StatusCode)
	}
	return nil
}

func (r *runner) get(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.host+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Basic "+r.credentials)

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("read returned %d", resp.StatusCode)
	}
	return nil
}

// sampler observes storage and container resources during a run.
type sampler struct {
	pool      *pgxpool.Pool
	redis     *redis.Client
	container string
}

const ingestionQueueKey = "obsevo:ingestion:queue"

func newSampler(ctx context.Context, databaseURL, redisURL, container string) (*sampler, error) {
	s := &sampler{container: container}

	var problems []string
	if databaseURL != "" {
		pool, err := pgxpool.New(ctx, databaseURL)
		if err != nil {
			problems = append(problems, "postgres: "+err.Error())
		} else {
			s.pool = pool
		}
	}
	if redisURL != "" {
		addr := strings.TrimPrefix(strings.TrimPrefix(redisURL, "redis://"), "rediss://")
		s.redis = redis.NewClient(&redis.Options{Addr: addr})
		if err := s.redis.Ping(ctx).Err(); err != nil {
			problems = append(problems, "redis: "+err.Error())
			s.redis = nil
		}
	}

	if len(problems) > 0 {
		return s, fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return s, nil
}

func (s *sampler) close() {
	if s == nil {
		return
	}
	if s.pool != nil {
		s.pool.Close()
	}
	if s.redis != nil {
		_ = s.redis.Close()
	}
}

func (s *sampler) snapshot(ctx context.Context) Snapshot {
	var snapshot Snapshot
	if s == nil {
		return snapshot
	}

	if s.pool != nil {
		_ = s.pool.QueryRow(ctx, "SELECT pg_database_size(current_database())").Scan(&snapshot.PostgresBytes)
		_ = s.pool.QueryRow(ctx, "SELECT count(*) FROM traces").Scan(&snapshot.TraceCount)
		_ = s.pool.QueryRow(ctx, "SELECT count(*) FROM observations").Scan(&snapshot.ObservationCount)
	}
	if s.redis != nil {
		snapshot.QueueDepth, _ = s.redis.LLen(ctx, ingestionQueueKey).Result()
		snapshot.RedisMemoryBytes = redisUsedMemory(ctx, s.redis)
	}

	return snapshot
}

// startSampling polls resources until the returned function is called, which
// reports the peaks observed.
func (s *sampler) startSampling(ctx context.Context) func() (float64, float64, int64) {
	stop := make(chan struct{})
	done := make(chan struct{})

	var peakCPU, peakMemory float64
	var peakQueue int64

	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			if s != nil && s.redis != nil {
				if depth, err := s.redis.LLen(ctx, ingestionQueueKey).Result(); err == nil && depth > peakQueue {
					peakQueue = depth
				}
			}
			if s != nil && s.container != "" {
				if cpu, memory, err := dockerStats(s.container); err == nil {
					if cpu > peakCPU {
						peakCPU = cpu
					}
					if memory > peakMemory {
						peakMemory = memory
					}
				}
			}
		}
	}()

	return func() (float64, float64, int64) {
		close(stop)
		<-done
		return peakCPU, peakMemory, peakQueue
	}
}

// waitForDrain blocks until the ingestion queue is empty, returning how long
// the worker took. This is the worker latency the brief asks for: the lag
// between an event being accepted and it being queryable.
func (s *sampler) waitForDrain(ctx context.Context, timeout time.Duration) float64 {
	if s == nil || s.redis == nil {
		return 0
	}

	started := time.Now()
	deadline := time.After(timeout)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return time.Since(started).Seconds()
		case <-deadline:
			log.Printf("worker did not drain within %s", timeout)
			return time.Since(started).Seconds()
		case <-ticker.C:
			depth, err := s.redis.LLen(ctx, ingestionQueueKey).Result()
			if err != nil || depth == 0 {
				return time.Since(started).Seconds()
			}
		}
	}
}

// redisUsedMemory reads used_memory out of INFO MEMORY.
func redisUsedMemory(ctx context.Context, client *redis.Client) int64 {
	info, err := client.Info(ctx, "memory").Result()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(info, "\n") {
		if strings.HasPrefix(line, "used_memory:") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "used_memory:"))
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err == nil {
				return parsed
			}
		}
	}
	return 0
}

// dockerStats reads one container's CPU percentage and memory in MB.
func dockerStats(container string) (float64, float64, error) {
	out, err := exec.Command("docker", "stats", "--no-stream",
		"--format", "{{.CPUPerc}} {{.MemUsage}}", container).Output()
	if err != nil {
		return 0, 0, err
	}

	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return 0, 0, fmt.Errorf("unexpected docker stats output: %q", out)
	}

	cpu, err := strconv.ParseFloat(strings.TrimSuffix(fields[0], "%"), 64)
	if err != nil {
		return 0, 0, err
	}
	return cpu, parseMemory(fields[1]), nil
}

// parseMemory converts docker's "123.4MiB" style value to MB.
func parseMemory(value string) float64 {
	multipliers := []struct {
		suffix string
		factor float64
	}{
		{"GiB", 1024}, {"MiB", 1}, {"KiB", 1.0 / 1024},
		{"GB", 1000}, {"MB", 1}, {"kB", 0.001},
	}

	for _, m := range multipliers {
		if strings.HasSuffix(value, m.suffix) {
			parsed, err := strconv.ParseFloat(strings.TrimSuffix(value, m.suffix), 64)
			if err != nil {
				return 0
			}
			return parsed * m.factor
		}
	}
	return 0
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	index := int(p * float64(len(sorted)-1))
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func randomID() string {
	const hex = "0123456789abcdef"
	buf := make([]byte, 32)
	for i := range buf {
		buf[i] = hex[rand.Intn(len(hex))]
	}
	return string(buf)
}

func (r Report) print() {
	fmt.Printf("\n=== %s ===\n", r.Scenario)
	fmt.Printf("duration            %.1fs\n", r.DurationSeconds)
	fmt.Printf("traces sent         %d (%d events in %d requests)\n", r.TracesSent, r.EventsSent, r.RequestsSent)
	fmt.Printf("achieved rate       %.0f traces/min (%.0f events/s)\n", r.AchievedTracesPerMinute, r.AchievedEventsPerSecond)
	fmt.Printf("failures            %d\n", r.Failures)
	fmt.Printf("\ningestion latency   p50 %.1fms  p95 %.1fms  p99 %.1fms  max %.1fms\n",
		r.IngestP50Ms, r.IngestP95Ms, r.IngestP99Ms, r.IngestMaxMs)
	fmt.Printf("read latency        p50 %.1fms  p95 %.1fms\n", r.ReadP50Ms, r.ReadP95Ms)
	fmt.Printf("worker drain        %.2fs after the last request (peak queue %d)\n",
		r.WorkerDrainSeconds, r.PeakQueueDepth)

	if r.After.PostgresBytes > 0 {
		grown := r.After.PostgresBytes - r.Before.PostgresBytes
		fmt.Printf("\npostgres size       %.1f MB (+%.1f MB this run)\n",
			float64(r.After.PostgresBytes)/(1<<20), float64(grown)/(1<<20))
		if r.EventsSent > 0 {
			fmt.Printf("bytes per event     %.0f\n", float64(grown)/float64(r.EventsSent))
		}
		fmt.Printf("rows                %d traces, %d observations\n",
			r.After.TraceCount, r.After.ObservationCount)
	}
	if r.After.RedisMemoryBytes > 0 {
		fmt.Printf("redis memory        %.1f MB\n", float64(r.After.RedisMemoryBytes)/(1<<20))
	}
	if r.PeakCPUPercent > 0 || r.PeakMemoryMB > 0 {
		fmt.Printf("api container       peak CPU %.1f%%, peak memory %.0f MB\n", r.PeakCPUPercent, r.PeakMemoryMB)
	}
	fmt.Println()
}
