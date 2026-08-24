package worker

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/langfuse-light/langfuse-light/internal/queue"
	"github.com/langfuse-light/langfuse-light/internal/services"
)

// Worker processes items from the ingestion queue.
type Worker struct {
	queue        *queue.Queue
	traceService *services.TraceService
	interval     time.Duration
	stopCh       chan struct{}
}

// NewWorker creates a new ingestion worker.
func NewWorker(q *queue.Queue, ts *services.TraceService, interval time.Duration) *Worker {
	return &Worker{
		queue:        q,
		traceService: ts,
		interval:     interval,
		stopCh:       make(chan struct{}),
	}
}

// Start begins processing queue items in the background.
func (w *Worker) Start(ctx context.Context) {
	log.Println("Starting ingestion worker...")
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Ingestion worker stopping due to context cancellation")
			return
		case <-w.stopCh:
			log.Println("Ingestion worker stopped")
			return
		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

// Stop signals the worker to stop.
func (w *Worker) Stop() {
	close(w.stopCh)
}

func (w *Worker) processBatch(ctx context.Context) {
	for {
		item, err := w.queue.Dequeue(ctx)
		if err != nil {
			log.Printf("Error dequeuing item: %v", err)
			return
		}
		if item == nil {
			return
		}

		if err := w.processItem(ctx, item); err != nil {
			log.Printf("Error processing item %s: %v", item.ID, err)
			_ = w.queue.RequeueFailed(ctx, *item)
		}
	}
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
		_, err := w.traceService.CreateObservation(ctx, req)
		return err
	default:
		log.Printf("Unknown ingestion item type: %s", item.Type)
		return nil
	}
}
