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
	processingKey     = "langfuse:ingestion:processing"
)

// IngestionItem represents an item in the ingestion queue.
type IngestionItem struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	ProjectID string          `json:"project_id"`
	CreatedAt time.Time       `json:"created_at"`
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

// QueueLen returns the number of items in the ingestion queue.
func (q *Queue) QueueLen(ctx context.Context) (int64, error) {
	return q.client.LLen(ctx, ingestionQueueKey).Result()
}

// RequeueFailed re-enqueues a failed item for retry.
func (q *Queue) RequeueFailed(ctx context.Context, item IngestionItem) error {
	return q.Enqueue(ctx, item)
}
