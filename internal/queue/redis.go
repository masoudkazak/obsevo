package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	ingestionQueueKey = "langfuse:ingestion:queue"
	deadLetterKey     = "langfuse:ingestion:dead"

	// maxAttempts caps redelivery so a permanently malformed item cannot spin
	// in the queue forever.
	maxAttempts = 5

	// deadLetterLimit bounds the dead-letter list so it cannot grow without end.
	deadLetterLimit = 1000
)

// IngestionItem represents an item in the ingestion queue.
type IngestionItem struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	ProjectID string          `json:"project_id"`
	CreatedAt time.Time       `json:"created_at"`
	Attempts  int             `json:"attempts"`
	LastError string          `json:"last_error,omitempty"`
}

// Queue provides Redis-backed ingestion queue operations.
type Queue struct {
	client *redis.Client
}

// NewQueue creates a new Redis-backed queue.
func NewQueue(client *redis.Client) *Queue {
	return &Queue{client: client}
}

// Enqueue adds an item to the ingestion queue.
func (q *Queue) Enqueue(ctx context.Context, item IngestionItem) error {
	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("marshaling ingestion item: %w", err)
	}

	if err := q.client.RPush(ctx, ingestionQueueKey, data).Err(); err != nil {
		return fmt.Errorf("pushing to ingestion queue: %w", err)
	}

	return nil
}

// Dequeue removes and returns an item from the ingestion queue.
// Returns nil if the queue is empty.
func (q *Queue) Dequeue(ctx context.Context) (*IngestionItem, error) {
	data, err := q.client.LPop(ctx, ingestionQueueKey).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("popping from ingestion queue: %w", err)
	}

	var item IngestionItem
	if err := json.Unmarshal(data, &item); err != nil {
		return nil, fmt.Errorf("unmarshaling ingestion item: %w", err)
	}

	return &item, nil
}

// DequeueBlocking waits up to timeout for an item to become available.
// Returns nil when the timeout elapses with an empty queue, which lets the
// worker react to new events immediately instead of polling on a ticker.
func (q *Queue) DequeueBlocking(ctx context.Context, timeout time.Duration) (*IngestionItem, error) {
	res, err := q.client.BLPop(ctx, timeout, ingestionQueueKey).Result()
	if err != nil {
		if err == redis.Nil || ctx.Err() != nil {
			return nil, nil
		}
		return nil, fmt.Errorf("blocking pop from ingestion queue: %w", err)
	}
	// BLPop returns [key, value].
	if len(res) < 2 {
		return nil, nil
	}

	var item IngestionItem
	if err := json.Unmarshal([]byte(res[1]), &item); err != nil {
		return nil, fmt.Errorf("unmarshaling ingestion item: %w", err)
	}

	return &item, nil
}

// EnqueueBatch pushes several items in one round trip.
func (q *Queue) EnqueueBatch(ctx context.Context, items []IngestionItem) error {
	if len(items) == 0 {
		return nil
	}

	payloads := make([]interface{}, 0, len(items))
	for _, item := range items {
		data, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("marshaling ingestion item: %w", err)
		}
		payloads = append(payloads, data)
	}

	if err := q.client.RPush(ctx, ingestionQueueKey, payloads...).Err(); err != nil {
		return fmt.Errorf("pushing batch to ingestion queue: %w", err)
	}
	return nil
}

// QueueLen returns the number of items in the ingestion queue.
func (q *Queue) QueueLen(ctx context.Context) (int64, error) {
	return q.client.LLen(ctx, ingestionQueueKey).Result()
}

// RequeueFailed re-enqueues a failed item, or moves it to the dead-letter list
// once it has exhausted its attempts. It reports whether the item was retried.
func (q *Queue) RequeueFailed(ctx context.Context, item IngestionItem, cause error) (bool, error) {
	item.Attempts++
	if cause != nil {
		item.LastError = cause.Error()
	}

	if item.Attempts >= maxAttempts {
		return false, q.deadLetter(ctx, item)
	}

	return true, q.Enqueue(ctx, item)
}

// deadLetter parks an item that cannot be processed, keeping the most recent
// deadLetterLimit entries for inspection.
func (q *Queue) deadLetter(ctx context.Context, item IngestionItem) error {
	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("marshaling dead-letter item: %w", err)
	}
	pipe := q.client.TxPipeline()
	pipe.RPush(ctx, deadLetterKey, data)
	pipe.LTrim(ctx, deadLetterKey, -deadLetterLimit, -1)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("writing to dead-letter queue: %w", err)
	}
	return nil
}

// DeadLetterLen returns the number of items parked in the dead-letter list.
func (q *Queue) DeadLetterLen(ctx context.Context) (int64, error) {
	return q.client.LLen(ctx, deadLetterKey).Result()
}
