package services

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/langfuse-light/langfuse-light/internal/db"
)

// EvaluationService handles scoring and analytics business logic.
type EvaluationService struct {
	queries *db.Queries
}

// NewEvaluationService creates a new evaluation service.
func NewEvaluationService(queries *db.Queries) *EvaluationService {
	return &EvaluationService{queries: queries}
}

// CreateScoreRequest is the request body for creating a score.
type CreateScoreRequest struct {
	TraceID string  `json:"trace_id"`
	Name    string  `json:"name"`
	Value   float64 `json:"value"`
	Comment string  `json:"comment"`
	Source  string  `json:"source"`
	UserID  string  `json:"user_id"`
}

// CreateScore creates a new score for a trace.
func (s *EvaluationService) CreateScore(ctx context.Context, req CreateScoreRequest) (db.Score, error) {
	if req.TraceID == "" {
		return db.Score{}, fmt.Errorf("trace_id is required")
	}
	if req.Name == "" {
		return db.Score{}, fmt.Errorf("name is required")
	}
	if req.Source == "" {
		req.Source = "USER"
	}

	score, err := s.queries.CreateScore(ctx, db.CreateScoreParams{
		TraceID: req.TraceID,
		Name:    req.Name,
		Value:   pgtype.Float8{Float64: req.Value, Valid: true},
		Comment: pgtype.Text{String: req.Comment, Valid: req.Comment != ""},
		Source:  req.Source,
		UserID:  pgtype.Text{String: req.UserID, Valid: req.UserID != ""},
	})
	if err != nil {
		return db.Score{}, fmt.Errorf("creating score: %w", err)
	}

	return score, nil
}

// GetScore returns a score by ID.
func (s *EvaluationService) GetScore(ctx context.Context, id string) (db.Score, error) {
	score, err := s.queries.GetScoreByID(ctx, id)
	if err != nil {
		return db.Score{}, fmt.Errorf("getting score: %w", err)
	}
	return score, nil
}

// ListScoresByTrace returns all scores for a trace.
func (s *EvaluationService) ListScoresByTrace(ctx context.Context, traceID string) ([]db.Score, error) {
	scores, err := s.queries.GetScoresByTraceID(ctx, traceID)
	if err != nil {
		return nil, fmt.Errorf("listing scores: %w", err)
	}
	return scores, nil
}

// ListScoresByTraceAndName returns scores for a trace filtered by name.
func (s *EvaluationService) ListScoresByTraceAndName(ctx context.Context, traceID, name string) ([]db.Score, error) {
	scores, err := s.queries.GetScoresByTraceIDAndName(ctx, db.GetScoresByTraceIDAndNameParams{
		TraceID: traceID,
		Name:    name,
	})
	if err != nil {
		return nil, fmt.Errorf("listing scores by name: %w", err)
	}
	return scores, nil
}

// ScoreAggregation represents aggregated score statistics.
type ScoreAggregation struct {
	Name     string  `json:"name"`
	Count    int64   `json:"count"`
	AvgValue float64 `json:"avg_value"`
	MinValue float64 `json:"min_value"`
	MaxValue float64 `json:"max_value"`
}

// GetScoreAggregations returns aggregated score statistics for a project.
func (s *EvaluationService) GetScoreAggregations(ctx context.Context, projectID string) ([]ScoreAggregation, error) {
	rows, err := s.queries.GetScoreAggregationByProjectID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("getting score aggregations: %w", err)
	}

	result := make([]ScoreAggregation, len(rows))
	for i, row := range rows {
		minVal := 0.0
		maxVal := 0.0
		if v, ok := row.MinValue.(float64); ok {
			minVal = v
		}
		if v, ok := row.MaxValue.(float64); ok {
			maxVal = v
		}
		result[i] = ScoreAggregation{
			Name:     row.Name,
			Count:    row.Count,
			AvgValue: row.AvgValue,
			MinValue: minVal,
			MaxValue: maxVal,
		}
	}

	return result, nil
}

// LatencyStats represents latency statistics for a project.
type LatencyStats struct {
	TotalTraces       int64   `json:"total_traces"`
	AvgLatencySeconds float64 `json:"avg_latency_seconds"`
	MinLatencySeconds float64 `json:"min_latency_seconds"`
	MaxLatencySeconds float64 `json:"max_latency_seconds"`
}

// GetLatencyStats returns latency statistics for a project.
func (s *EvaluationService) GetLatencyStats(ctx context.Context, projectID string) (LatencyStats, error) {
	row, err := s.queries.GetTraceLatencyStatsByProjectID(ctx, projectID)
	if err != nil {
		return LatencyStats{}, fmt.Errorf("getting latency stats: %w", err)
	}

	minLat := 0.0
	maxLat := 0.0
	if v, ok := row.MinLatencySeconds.(float64); ok {
		minLat = v
	}
	if v, ok := row.MaxLatencySeconds.(float64); ok {
		maxLat = v
	}

	return LatencyStats{
		TotalTraces:       row.TotalTraces,
		AvgLatencySeconds: row.AvgLatencySeconds,
		MinLatencySeconds: minLat,
		MaxLatencySeconds: maxLat,
	}, nil
}

// CostStats represents cost statistics for a project.
type CostStats struct {
	TotalTraces int64   `json:"total_traces"`
	TotalCost   float64 `json:"total_cost"`
	AvgCost     float64 `json:"avg_cost"`
	MinCost     float64 `json:"min_cost"`
	MaxCost     float64 `json:"max_cost"`
}

// GetCostStats returns cost statistics for a project.
func (s *EvaluationService) GetCostStats(ctx context.Context, projectID string) (CostStats, error) {
	row, err := s.queries.GetTraceCostStatsByProjectID(ctx, projectID)
	if err != nil {
		return CostStats{}, fmt.Errorf("getting cost stats: %w", err)
	}

	minCost := 0.0
	maxCost := 0.0
	if v, ok := row.MinCost.(float64); ok {
		minCost = v
	}
	if v, ok := row.MaxCost.(float64); ok {
		maxCost = v
	}

	return CostStats{
		TotalTraces: row.TotalTraces,
		TotalCost:   float64(row.TotalCost),
		AvgCost:     row.AvgCost,
		MinCost:     minCost,
		MaxCost:     maxCost,
	}, nil
}

// TokenUsageStats represents token usage statistics for a project.
type TokenUsageStats struct {
	TotalTraces       int64   `json:"total_traces"`
	TotalTokens       int64   `json:"total_tokens"`
	AvgTokens         float64 `json:"avg_tokens"`
	TotalInputTokens  int64   `json:"total_input_tokens"`
	TotalOutputTokens int64   `json:"total_output_tokens"`
}

// GetTokenUsageStats returns token usage statistics for a project.
func (s *EvaluationService) GetTokenUsageStats(ctx context.Context, projectID string) (TokenUsageStats, error) {
	row, err := s.queries.GetTraceTokenUsageStatsByProjectID(ctx, projectID)
	if err != nil {
		return TokenUsageStats{}, fmt.Errorf("getting token usage stats: %w", err)
	}

	return TokenUsageStats{
		TotalTraces:       row.TotalTraces,
		TotalTokens:       row.TotalTokens,
		AvgTokens:         row.AvgTokens,
		TotalInputTokens:  row.TotalInputTokens,
		TotalOutputTokens: row.TotalOutputTokens,
	}, nil
}

// ErrorRateStats represents error rate statistics for a project.
type ErrorRateStats struct {
	TotalTraces int64   `json:"total_traces"`
	ErrorTraces int64   `json:"error_traces"`
	ErrorRate   float64 `json:"error_rate"`
}

// GetErrorRateStats returns error rate statistics for a project.
func (s *EvaluationService) GetErrorRateStats(ctx context.Context, projectID string) (ErrorRateStats, error) {
	row, err := s.queries.GetTraceErrorRateByProjectID(ctx, projectID)
	if err != nil {
		return ErrorRateStats{}, fmt.Errorf("getting error rate stats: %w", err)
	}

	errorRate := 0.0
	if row.TotalTraces > 0 {
		errorRate = float64(row.ErrorTraces) / float64(row.TotalTraces)
	}

	return ErrorRateStats{
		TotalTraces: row.TotalTraces,
		ErrorTraces: row.ErrorTraces,
		ErrorRate:   errorRate,
	}, nil
}

// AnalyticsSummary represents a full analytics summary for a project.
type AnalyticsSummary struct {
	Latency    LatencyStats       `json:"latency"`
	Cost       CostStats          `json:"cost"`
	TokenUsage TokenUsageStats    `json:"token_usage"`
	ErrorRate  ErrorRateStats     `json:"error_rate"`
	Scores     []ScoreAggregation `json:"scores"`
}

// GetAnalyticsSummary returns a complete analytics summary for a project.
func (s *EvaluationService) GetAnalyticsSummary(ctx context.Context, projectID string) (AnalyticsSummary, error) {
	latency, err := s.GetLatencyStats(ctx, projectID)
	if err != nil {
		latency = LatencyStats{}
	}

	cost, err := s.GetCostStats(ctx, projectID)
	if err != nil {
		cost = CostStats{}
	}

	tokenUsage, err := s.GetTokenUsageStats(ctx, projectID)
	if err != nil {
		tokenUsage = TokenUsageStats{}
	}

	errorRate, err := s.GetErrorRateStats(ctx, projectID)
	if err != nil {
		errorRate = ErrorRateStats{}
	}

	scores, err := s.GetScoreAggregations(ctx, projectID)
	if err != nil {
		scores = []ScoreAggregation{}
	}

	return AnalyticsSummary{
		Latency:    latency,
		Cost:       cost,
		TokenUsage: tokenUsage,
		ErrorRate:  errorRate,
		Scores:     scores,
	}, nil
}
