// Command migrate-langfuse copies a project from a Langfuse instance into
// Obsevo.
//
// It reads through Langfuse's own public API and writes through this project's
// Langfuse-compatible API, so the transform is mostly an identity mapping — that
// compatibility is the point of the data model. Where a field has no home here,
// the tool says so rather than dropping it silently.
//
//	go run ./cmd/migrate-langfuse \
//	  -source-host https://cloud.langfuse.com \
//	  -source-public-key pk-lf-... -source-secret-key sk-lf-... \
//	  -target-host http://localhost:3001 \
//	  -target-public-key pk-lf-... -target-secret-key sk-lf-... \
//	  -what traces,scores,prompts,datasets
//
// Run with -dry-run first: it reads everything and reports what would be
// written without touching the target.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// writeBatchSize is how many events are pushed per ingestion request.
const writeBatchSize = 50

func main() {
	var (
		sourceHost      = flag.String("source-host", "https://cloud.langfuse.com", "Langfuse instance to read from")
		sourcePublicKey = flag.String("source-public-key", os.Getenv("LANGFUSE_SOURCE_PUBLIC_KEY"), "source public key")
		sourceSecretKey = flag.String("source-secret-key", os.Getenv("LANGFUSE_SOURCE_SECRET_KEY"), "source secret key")

		targetHost      = flag.String("target-host", "http://localhost:3001", "Obsevo instance to write to")
		targetPublicKey = flag.String("target-public-key", os.Getenv("LANGFUSE_PUBLIC_KEY"), "target public key")
		targetSecretKey = flag.String("target-secret-key", os.Getenv("LANGFUSE_SECRET_KEY"), "target secret key")

		what     = flag.String("what", "traces,scores,prompts,datasets", "comma-separated: traces, scores, prompts, datasets")
		fromTime = flag.String("from", "", "only migrate traces at or after this RFC 3339 timestamp")
		toTime   = flag.String("to", "", "only migrate traces before this RFC 3339 timestamp")
		pageSize = flag.Int("page-size", 50, "records fetched per request")
		maxPages = flag.Int("max-pages", 0, "stop after this many pages per resource (0 = no limit)")
		idPrefix = flag.String("id-prefix", "", "prefix every migrated trace, observation and score id (use when ids could collide on the target)")
		dryRun   = flag.Bool("dry-run", false, "read and report without writing")
	)
	flag.Parse()

	if *sourcePublicKey == "" || *sourceSecretKey == "" {
		log.Fatal("source credentials are required (-source-public-key / -source-secret-key)")
	}
	if !*dryRun && (*targetPublicKey == "" || *targetSecretKey == "") {
		log.Fatal("target credentials are required unless -dry-run is set")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	m := &migrator{
		source:   newClient(*sourceHost, *sourcePublicKey, *sourceSecretKey),
		target:   newClient(*targetHost, *targetPublicKey, *targetSecretKey),
		pageSize: *pageSize,
		maxPages: *maxPages,
		fromTime: *fromTime,
		toTime:   *toTime,
		idPrefix: *idPrefix,
		dryRun:   *dryRun,
	}

	if *dryRun {
		log.Println("dry run: nothing will be written")
	}

	selected := make(map[string]bool)
	for _, name := range strings.Split(*what, ",") {
		selected[strings.TrimSpace(name)] = true
	}

	var failed bool
	if selected["traces"] {
		if err := m.migrateTraces(ctx); err != nil {
			log.Printf("traces: %v", err)
			failed = true
		}
	}
	if selected["scores"] {
		if err := m.migrateScores(ctx); err != nil {
			log.Printf("scores: %v", err)
			failed = true
		}
	}
	if selected["prompts"] {
		if err := m.migratePrompts(ctx); err != nil {
			log.Printf("prompts: %v", err)
			failed = true
		}
	}
	if selected["datasets"] {
		if err := m.migrateDatasets(ctx); err != nil {
			log.Printf("datasets: %v", err)
			failed = true
		}
	}

	m.report()
	if failed {
		os.Exit(1)
	}
}

// migrator copies records between two Langfuse-compatible APIs.
type migrator struct {
	source   *client
	target   *client
	pageSize int
	maxPages int
	fromTime string
	toTime   string
	idPrefix string
	dryRun   bool

	counts   map[string]int
	warnings []string
}

func (m *migrator) count(resource string, n int) {
	if m.counts == nil {
		m.counts = make(map[string]int)
	}
	m.counts[resource] += n
}

func (m *migrator) warn(format string, args ...interface{}) {
	m.warnings = append(m.warnings, fmt.Sprintf(format, args...))
}

func (m *migrator) report() {
	fmt.Println("\n=== migration summary ===")
	for _, resource := range []string{"traces", "observations", "scores", "prompts", "datasets", "dataset items"} {
		if n, ok := m.counts[resource]; ok {
			fmt.Printf("  %-14s %d\n", resource, n)
		}
	}
	if len(m.warnings) > 0 {
		fmt.Println("\nnotes:")
		for _, warning := range m.warnings {
			fmt.Printf("  - %s\n", warning)
		}
	}
	if m.dryRun {
		fmt.Println("\n(dry run — nothing was written)")
	}
}

// --- traces and observations ------------------------------------------------

func (m *migrator) migrateTraces(ctx context.Context) error {
	log.Println("migrating traces and observations...")

	for page := 1; m.maxPages == 0 || page <= m.maxPages; page++ {
		params := url.Values{}
		params.Set("page", fmt.Sprint(page))
		params.Set("limit", fmt.Sprint(m.pageSize))
		if m.fromTime != "" {
			params.Set("fromTimestamp", m.fromTime)
		}
		if m.toTime != "" {
			params.Set("toTimestamp", m.toTime)
		}

		var response struct {
			Data []map[string]interface{} `json:"data"`
			Meta struct {
				TotalPages int `json:"totalPages"`
			} `json:"meta"`
		}
		if err := m.source.get(ctx, "/api/public/traces?"+params.Encode(), &response); err != nil {
			return fmt.Errorf("listing traces (page %d): %w", page, err)
		}
		if len(response.Data) == 0 {
			break
		}

		var events []map[string]interface{}
		for _, trace := range response.Data {
			traceID, _ := trace["id"].(string)
			if traceID == "" {
				continue
			}

			body := traceEventBody(trace)
			m.rewriteIDs(body, "id")
			events = append(events, event("trace-create", body))
			m.count("traces", 1)

			observations, err := m.fetchObservations(ctx, traceID)
			if err != nil {
				m.warn("trace %s: could not read observations: %v", traceID, err)
				continue
			}
			for _, observation := range observations {
				m.rewriteIDs(observation, "id", "traceId", "parentObservationId")
				events = append(events, event(observationEventType(observation), observation))
				m.count("observations", 1)
			}

			if len(events) >= writeBatchSize {
				if err := m.push(ctx, events); err != nil {
					return err
				}
				events = nil
			}
		}

		if err := m.push(ctx, events); err != nil {
			return err
		}

		if response.Meta.TotalPages > 0 && page >= response.Meta.TotalPages {
			break
		}
	}

	return nil
}

// fetchObservations reads a trace's observations, preferring the ones embedded
// in the trace response and falling back to the observations endpoint.
func (m *migrator) fetchObservations(ctx context.Context, traceID string) ([]map[string]interface{}, error) {
	var detail struct {
		Observations []map[string]interface{} `json:"observations"`
	}
	if err := m.source.get(ctx, "/api/public/traces/"+url.PathEscape(traceID), &detail); err == nil &&
		len(detail.Observations) > 0 {
		return detail.Observations, nil
	}

	var response struct {
		Data []map[string]interface{} `json:"data"`
	}
	params := url.Values{}
	params.Set("traceId", traceID)
	params.Set("limit", "500")
	if err := m.source.get(ctx, "/api/public/observations?"+params.Encode(), &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

// traceEventBody maps a Langfuse trace record onto an ingestion body.
// Langfuse names the trace's clock field `timestamp`; the rest carries over
// unchanged because both sides use the same field names.
func traceEventBody(trace map[string]interface{}) map[string]interface{} {
	body := map[string]interface{}{}
	for _, field := range []string{
		"id", "name", "userId", "sessionId", "input", "output", "metadata",
		"tags", "release", "version", "public", "environment", "timestamp",
	} {
		if value, ok := trace[field]; ok && value != nil {
			body[field] = value
		}
	}

	// Older Langfuse payloads used a different name for the same field.
	if _, ok := body["timestamp"]; !ok {
		if value, ok := trace["startTime"]; ok {
			body["timestamp"] = value
		}
	}
	if _, ok := body["sessionId"]; !ok {
		if value, ok := trace["sessionId"]; ok {
			body["sessionId"] = value
		}
	}

	return body
}

// observationEventType maps an observation's type onto the ingestion event that
// creates it.
func observationEventType(observation map[string]interface{}) string {
	switch strings.ToUpper(fmt.Sprint(observation["type"])) {
	case "GENERATION":
		return "generation-create"
	case "EVENT":
		return "event-create"
	default:
		return "span-create"
	}
}

// --- scores -----------------------------------------------------------------

func (m *migrator) migrateScores(ctx context.Context) error {
	log.Println("migrating scores...")

	for page := 1; m.maxPages == 0 || page <= m.maxPages; page++ {
		params := url.Values{}
		params.Set("page", fmt.Sprint(page))
		params.Set("limit", fmt.Sprint(m.pageSize))

		var response struct {
			Data []map[string]interface{} `json:"data"`
			Meta struct {
				TotalPages int `json:"totalPages"`
			} `json:"meta"`
		}
		if err := m.source.get(ctx, "/api/public/scores?"+params.Encode(), &response); err != nil {
			return fmt.Errorf("listing scores (page %d): %w", page, err)
		}
		if len(response.Data) == 0 {
			break
		}

		var events []map[string]interface{}
		for _, score := range response.Data {
			m.rewriteIDs(score, "id", "traceId", "observationId")
			events = append(events, event("score-create", score))
			m.count("scores", 1)
		}
		if err := m.push(ctx, events); err != nil {
			return err
		}

		if response.Meta.TotalPages > 0 && page >= response.Meta.TotalPages {
			break
		}
	}

	return nil
}

// --- prompts ----------------------------------------------------------------

func (m *migrator) migratePrompts(ctx context.Context) error {
	log.Println("migrating prompts...")

	var listing struct {
		Data []struct {
			Name     string   `json:"name"`
			Versions []int    `json:"versions"`
			Labels   []string `json:"labels"`
		} `json:"data"`
	}
	if err := m.source.get(ctx, "/api/public/v2/prompts?limit=100", &listing); err != nil {
		return fmt.Errorf("listing prompts: %w", err)
	}

	for _, entry := range listing.Data {
		versions := entry.Versions
		if len(versions) == 0 {
			// Some Langfuse versions omit the version list; fetch production only.
			versions = []int{0}
		}

		for _, version := range versions {
			path := "/api/public/v2/prompts/" + url.PathEscape(entry.Name)
			if version > 0 {
				path += "?version=" + fmt.Sprint(version)
			}

			var prompt map[string]interface{}
			if err := m.source.get(ctx, path, &prompt); err != nil {
				m.warn("prompt %s v%d: %v", entry.Name, version, err)
				continue
			}

			body := map[string]interface{}{"name": entry.Name}
			for _, field := range []string{"prompt", "type", "config", "labels", "tags", "commitMessage"} {
				if value, ok := prompt[field]; ok && value != nil {
					body[field] = value
				}
			}

			if m.dryRun {
				m.count("prompts", 1)
				continue
			}
			if err := m.target.post(ctx, "/api/public/v2/prompts", body, nil); err != nil {
				m.warn("prompt %s v%d: %v", entry.Name, version, err)
				continue
			}
			m.count("prompts", 1)
		}
	}

	return nil
}

// --- datasets ---------------------------------------------------------------

func (m *migrator) migrateDatasets(ctx context.Context) error {
	log.Println("migrating datasets...")

	var listing struct {
		Data []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"data"`
	}
	if err := m.source.get(ctx, "/api/public/datasets?limit=100", &listing); err != nil {
		return fmt.Errorf("listing datasets: %w", err)
	}

	for _, dataset := range listing.Data {
		var detail struct {
			Items []map[string]interface{} `json:"items"`
		}
		if err := m.source.get(ctx, "/api/public/datasets/"+url.PathEscape(dataset.Name), &detail); err != nil {
			m.warn("dataset %s: %v", dataset.Name, err)
			continue
		}

		if !m.dryRun {
			err := m.target.post(ctx, "/api/public/datasets", map[string]interface{}{
				"name":        dataset.Name,
				"description": dataset.Description,
			}, nil)
			if err != nil {
				m.warn("dataset %s: %v", dataset.Name, err)
				continue
			}
		}
		m.count("datasets", 1)

		for _, item := range detail.Items {
			body := map[string]interface{}{"datasetName": dataset.Name}
			for _, field := range []string{
				"id", "input", "expectedOutput", "metadata",
				"sourceTraceId", "sourceObservationId", "status",
			} {
				if value, ok := item[field]; ok && value != nil {
					body[field] = value
				}
			}
			m.rewriteIDs(body, "id", "sourceTraceId", "sourceObservationId")

			if m.dryRun {
				m.count("dataset items", 1)
				continue
			}
			if err := m.target.post(ctx, "/api/public/dataset-items", body, nil); err != nil {
				m.warn("dataset %s item: %v", dataset.Name, err)
				continue
			}
			m.count("dataset items", 1)
		}

		// Dataset runs reference traces that may not have been migrated, and
		// their aggregates are recomputed from run items here, so they are left
		// for the operator to re-run rather than copied half-formed.
		m.warn("dataset %s: runs were not migrated (re-run experiments against the migrated items)", dataset.Name)
	}

	return nil
}

// rewriteIDs applies the configured prefix to the named id fields, so records
// can be migrated into an instance that already holds those ids.
func (m *migrator) rewriteIDs(record map[string]interface{}, fields ...string) {
	if m.idPrefix == "" {
		return
	}
	for _, field := range fields {
		if value, ok := record[field].(string); ok && value != "" {
			record[field] = m.idPrefix + value
		}
	}
}

// --- transport --------------------------------------------------------------

func event(eventType string, body map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"id":        fmt.Sprintf("migrate-%d-%s", time.Now().UnixNano(), eventType),
		"type":      eventType,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"body":      body,
	}
}

// push writes a batch of ingestion events to the target.
func (m *migrator) push(ctx context.Context, events []map[string]interface{}) error {
	if len(events) == 0 || m.dryRun {
		return nil
	}

	var response struct {
		Errors []struct {
			ID      string `json:"id"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := m.target.post(ctx, "/api/public/ingestion", map[string]interface{}{"batch": events}, &response); err != nil {
		return fmt.Errorf("writing batch: %w", err)
	}

	for _, failure := range response.Errors {
		m.warn("event %s rejected: %s", failure.ID, failure.Message)
	}
	return nil
}

// client is a minimal Langfuse API client.
type client struct {
	host        string
	credentials string
	http        *http.Client
}

func newClient(host, publicKey, secretKey string) *client {
	return &client{
		host:        strings.TrimRight(host, "/"),
		credentials: base64.StdEncoding.EncodeToString([]byte(publicKey + ":" + secretKey)),
		http:        &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *client) get(ctx context.Context, path string, out interface{}) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

func (c *client) post(ctx context.Context, path string, body, out interface{}) error {
	return c.do(ctx, http.MethodPost, path, body, out)
}

func (c *client) do(ctx context.Context, method, path string, body, out interface{}) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.host+path, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Basic "+c.credentials)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s returned %d: %s", method, path, resp.StatusCode, truncate(string(raw), 300))
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
