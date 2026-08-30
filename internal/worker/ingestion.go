package worker

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/langfuse-light/langfuse-light/internal/queue"
	"github.com/langfuse-light/langfuse-light/internal/services"
)

const (
	// pollTimeout is how long a drained worker waits on Redis before looping.
	// It bounds shutdown latency; it is not a polling interval, because a new
	// item wakes the blocking pop immediately.
	pollTimeout = 1 * time.Second

	// aggregateFlushInterval is how often trace roll-ups are recomputed for the
	// traces touched since the last pass.
	aggregateFlushInterval = 2 * time.Second

	// aggregateFlushThreshold forces a roll-up flush during a sustained burst,
	// so trace totals do not lag far behind under continuous load.
	aggregateFlushThreshold = 500

	// shardBuffer is the per-shard queue depth. It absorbs a burst without the
	// dispatcher blocking on one slow shard.
	shardBuffer = 256
)

// Worker consumes the ingestion queue and writes to Postgres.
//
// One goroutine reads Redis and fans items out to a pool of shards. Events are
// routed by trace id, so every event belonging to a trace is handled by the
// same shard in arrival order — a span's create and its later update cannot be
// reordered — while unrelated traces are written concurrently. A single
// sequential writer cannot keep up past a few hundred events per second,
// because each event costs a database round trip.
type Worker struct {
	queue        *queue.Queue
	traceService *services.TraceService
	evalService  *services.EvaluationService
	costService  *services.CostService

	shards      []chan *queue.IngestionItem
	concurrency int

	stopCh chan struct{}
	doneCh chan struct{}

	// dirtyTraces collects the traces touched since the last flush so their
	// cost and token roll-ups are recomputed once per burst, not per event.
	dirtyMu     sync.Mutex
	dirtyTraces map[traceRef]struct{}
}

type traceRef struct {
	projectID string
	traceID   string
}

// NewWorker creates an ingestion worker. A concurrency of zero picks a default
// from the available CPUs.
func NewWorker(
	q *queue.Queue,
	ts *services.TraceService,
	es *services.EvaluationService,
	cs *services.CostService,
	concurrency int,
) *Worker {
	if concurrency <= 0 {
		concurrency = defaultConcurrency()
	}

	shards := make([]chan *queue.IngestionItem, concurrency)
	for i := range shards {
		shards[i] = make(chan *queue.IngestionItem, shardBuffer)
	}

	return &Worker{
		queue:        q,
		traceService: ts,
		evalService:  es,
		costService:  cs,
		shards:       shards,
		concurrency:  concurrency,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
		dirtyTraces:  make(map[traceRef]struct{}),
	}
}

// defaultConcurrency keeps a core free for the HTTP server and caps the pool so
// a small VPS does not exhaust the database connection pool.
func defaultConcurrency() int {
	concurrency := runtime.NumCPU() - 1
	if concurrency < 2 {
		concurrency = 2
	}
	if concurrency > 8 {
		concurrency = 8
	}
	return concurrency
}

// Start consumes the ingestion queue until the context is cancelled or Stop is
// called. It blocks on Redis rather than polling, so an enqueued event is
// picked up as soon as it lands.
func (w *Worker) Start(ctx context.Context) {
	log.Printf("Starting ingestion worker (%d shards)...", w.concurrency)
	defer close(w.doneCh)

	var shardWorkers sync.WaitGroup
	for i := range w.shards {
		shardWorkers.Add(1)
		go func(items <-chan *queue.IngestionItem) {
			defer shardWorkers.Done()
			for item := range items {
				w.handle(ctx, item)
			}
		}(w.shards[i])
	}

	flushDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(aggregateFlushInterval)
		defer ticker.Stop()
		for {
			select {
			case <-flushDone:
				return
			case <-ticker.C:
				w.flushAggregates(ctx)
			}
		}
	}()

	w.dispatch(ctx)
	close(flushDone)

	// Close the shards and let them finish what they hold before the final
	// roll-up, so no trace is left with stale totals.
	for _, shard := range w.shards {
		close(shard)
	}
	shardWorkers.Wait()

	// The parent context may already be cancelled during shutdown; the final
	// flush still needs to reach the database.
	final, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	w.flushAggregates(final)
}

// dispatch reads the queue and routes each item to its trace's shard.
func (w *Worker) dispatch(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Println("Ingestion worker stopping due to context cancellation")
			return
		case <-w.stopCh:
			log.Println("Ingestion worker stopped")
			return
		default:
		}

		item, err := w.queue.DequeueBlocking(ctx, pollTimeout)
		if err != nil {
			log.Printf("Error dequeuing item: %v", err)
			continue
		}
		if item == nil {
			continue
		}

		select {
		case w.shards[w.shardFor(item)] <- item:
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		}
	}
}

// shardFor routes an item by the trace it belongs to, so per-trace ordering is
// preserved. Items with no trace id are spread across shards by their own id.
func (w *Worker) shardFor(item *queue.IngestionItem) int {
	key := traceIDOf(item)
	if key == "" {
		key = item.ID
	}

	hash := fnv.New32a()
	_, _ = hash.Write([]byte(key))
	return int(hash.Sum32() % uint32(len(w.shards)))
}

// traceIDOf reads the trace id out of a queued payload without fully decoding it.
func traceIDOf(item *queue.IngestionItem) string {
	var envelope struct {
		ID      string `json:"id"`
		TraceID string `json:"trace_id"`
	}
	if err := json.Unmarshal(item.Payload, &envelope); err != nil {
		return ""
	}
	if envelope.TraceID != "" {
		return envelope.TraceID
	}
	// A trace event carries its own id, which is the trace id.
	if item.Type == "trace" {
		return envelope.ID
	}
	return ""
}

// handle processes one item, retrying or dead-lettering on failure.
func (w *Worker) handle(ctx context.Context, item *queue.IngestionItem) {
	if err := w.processItem(ctx, item); err != nil {
		retried, requeueErr := w.queue.RequeueFailed(ctx, *item, err)
		if requeueErr != nil {
			log.Printf("Error requeuing item %s: %v", item.ID, requeueErr)
		} else if !retried {
			log.Printf("Item %s exhausted retries, moved to dead-letter: %v", item.ID, err)
		}
	}

	w.dirtyMu.Lock()
	overThreshold := len(w.dirtyTraces) >= aggregateFlushThreshold
	w.dirtyMu.Unlock()

	if overThreshold {
		w.flushAggregates(ctx)
	}
}

// Stop signals the worker to stop and waits for it to drain what it holds.
func (w *Worker) Stop() {
	close(w.stopCh)
	select {
	case <-w.doneCh:
	case <-time.After(10 * time.Second):
		log.Println("Ingestion worker did not stop within 10s")
	}
}

// flushAggregates recomputes trace-level cost and token totals for every trace
// touched since the last flush, then clears the set.
func (w *Worker) flushAggregates(ctx context.Context) {
	w.dirtyMu.Lock()
	pending := w.dirtyTraces
	if len(pending) == 0 {
		w.dirtyMu.Unlock()
		return
	}
	w.dirtyTraces = make(map[traceRef]struct{})
	w.dirtyMu.Unlock()

	for ref := range pending {
		if err := w.traceService.RecalculateTraceAggregates(ctx, ref.projectID, ref.traceID); err != nil {
			log.Printf("Error recalculating aggregates for trace %s: %v", ref.traceID, err)
		}
	}
}

// markDirty records that a trace's roll-ups need recomputing.
func (w *Worker) markDirty(projectID, traceID string) {
	w.dirtyMu.Lock()
	w.dirtyTraces[traceRef{projectID: projectID, traceID: traceID}] = struct{}{}
	w.dirtyMu.Unlock()
}

func (w *Worker) processItem(ctx context.Context, item *queue.IngestionItem) error {
	switch item.Type {
	case "trace":
		var req services.CreateTraceRequest
		if err := json.Unmarshal(item.Payload, &req); err != nil {
			return err
		}
		_, err := w.traceService.CreateTrace(ctx, item.ProjectID, req)
		return err

	case "observation":
		var req services.CreateObservationRequest
		if err := json.Unmarshal(item.Payload, &req); err != nil {
			return err
		}
		obs, err := w.traceService.CreateObservation(ctx, item.ProjectID, req)
		if err != nil {
			return err
		}

		// Pricing runs here rather than on the request path so ingestion
		// latency never depends on it, and it reads the merged row because an
		// SDK sends the model on create and the token usage on update.
		if w.costService != nil {
			if _, err := w.costService.PriceObservation(ctx, item.ProjectID, obs); err != nil {
				log.Printf("Error pricing observation %s: %v", obs.ID, err)
			}
		}

		w.markDirty(item.ProjectID, obs.TraceID)
		return nil

	case "score":
		var req services.CreateScoreRequest
		if err := json.Unmarshal(item.Payload, &req); err != nil {
			return err
		}
		_, err := w.evalService.CreateScore(ctx, item.ProjectID, req)
		return err

	default:
		log.Printf("Unknown ingestion item type: %s", item.Type)
		return nil
	}
}
