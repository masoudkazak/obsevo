package services

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/langfuse-light/langfuse-light/internal/db"
)

// AnalyticsFilter narrows every analytics query to a slice of a project's data.
// An empty field means "no filter on this dimension".
type AnalyticsFilter struct {
	ProjectID   string
	UserID      string
	SessionID   string
	Environment string
	Tags        []string
	FromTime    string
	ToTime      string
}

// tags returns the tag filter as a non-nil slice, which the queries require.
func (f AnalyticsFilter) tags() []string {
	if f.Tags == nil {
		return []string{}
	}
	return f.Tags
}

// window resolves the filter's time range, defaulting to the last 30 days when
// the caller supplies neither bound.
func (f AnalyticsFilter) window() (time.Time, time.Time) {
	to := time.Now().UTC()
	if parsed := parseTime(f.ToTime); parsed.Valid {
		to = parsed.Time
	}

	from := to.AddDate(0, 0, -30)
	if parsed := parseTime(f.FromTime); parsed.Valid {
		from = parsed.Time
	}

	return from, to
}

// LatencyPercentiles reports the latency distribution, not just its mean.
// P95 is what tells a small team whether their users are waiting; an average
// hides that entirely.
type LatencyPercentiles struct {
	TotalTraces       int64   `json:"total_traces"`
	AvgLatencySeconds float64 `json:"avg_latency_seconds"`
	MinLatencySeconds float64 `json:"min_latency_seconds"`
	MaxLatencySeconds float64 `json:"max_latency_seconds"`
	P50LatencySeconds float64 `json:"p50_latency_seconds"`
	P90LatencySeconds float64 `json:"p90_latency_seconds"`
	P95LatencySeconds float64 `json:"p95_latency_seconds"`
	P99LatencySeconds float64 `json:"p99_latency_seconds"`
}

// GetLatencyPercentiles returns the latency distribution for a filtered slice.
func (s *EvaluationService) GetLatencyPercentiles(ctx context.Context, filter AnalyticsFilter) (LatencyPercentiles, error) {
	row, err := s.queries.GetTraceLatencyPercentiles(ctx, db.GetTraceLatencyPercentilesParams{
		ProjectID:   filter.ProjectID,
		UserID:      filter.UserID,
		SessionID:   filter.SessionID,
		Environment: filter.Environment,
		Tags:        filter.tags(),
		FromTime:    parseTime(filter.FromTime),
		ToTime:      parseTime(filter.ToTime),
	})
	if err != nil {
		return LatencyPercentiles{}, fmt.Errorf("getting latency percentiles: %w", err)
	}

	return LatencyPercentiles{
		TotalTraces:       row.TotalTraces,
		AvgLatencySeconds: row.AvgLatencySeconds,
		MinLatencySeconds: row.MinLatencySeconds,
		MaxLatencySeconds: row.MaxLatencySeconds,
		P50LatencySeconds: row.P50LatencySeconds,
		P90LatencySeconds: row.P90LatencySeconds,
		P95LatencySeconds: row.P95LatencySeconds,
		P99LatencySeconds: row.P99LatencySeconds,
	}, nil
}

// ModelUsage breaks cost and tokens down by model.
type ModelUsage struct {
	Model             string  `json:"model"`
	GenerationCount   int64   `json:"generation_count"`
	TotalCost         float64 `json:"total_cost"`
	AvgCost           float64 `json:"avg_cost"`
	InputTokens       int64   `json:"input_tokens"`
	OutputTokens      int64   `json:"output_tokens"`
	TotalTokens       int64   `json:"total_tokens"`
	AvgLatencySeconds float64 `json:"avg_latency_seconds"`
}

// GetModelUsage returns per-model cost, tokens and latency.
func (s *EvaluationService) GetModelUsage(ctx context.Context, filter AnalyticsFilter) ([]ModelUsage, error) {
	rows, err := s.queries.GetModelUsageStats(ctx, db.GetModelUsageStatsParams{
		ProjectID: filter.ProjectID,
		FromTime:  parseTime(filter.FromTime),
		ToTime:    parseTime(filter.ToTime),
	})
	if err != nil {
		return nil, fmt.Errorf("getting model usage: %w", err)
	}

	usage := make([]ModelUsage, 0, len(rows))
	for _, row := range rows {
		usage = append(usage, ModelUsage{
			Model:             row.Model,
			GenerationCount:   row.GenerationCount,
			TotalCost:         row.TotalCost,
			AvgCost:           row.AvgCost,
			InputTokens:       row.InputTokens,
			OutputTokens:      row.OutputTokens,
			TotalTokens:       row.TotalTokens,
			AvgLatencySeconds: row.AvgLatencySeconds,
		})
	}
	return usage, nil
}

// UserCost attributes spend to an end user.
type UserCost struct {
	UserID      string  `json:"user_id"`
	TraceCount  int64   `json:"trace_count"`
	TotalCost   float64 `json:"total_cost"`
	TotalTokens int64   `json:"total_tokens"`
}

// GetCostByUser returns the highest-spending users in the window.
func (s *EvaluationService) GetCostByUser(ctx context.Context, filter AnalyticsFilter, limit int32) ([]UserCost, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := s.queries.GetCostByUser(ctx, db.GetCostByUserParams{
		ProjectID: filter.ProjectID,
		FromTime:  parseTime(filter.FromTime),
		ToTime:    parseTime(filter.ToTime),
		Limit:     limit,
	})
	if err != nil {
		return nil, fmt.Errorf("getting cost by user: %w", err)
	}

	costs := make([]UserCost, 0, len(rows))
	for _, row := range rows {
		costs = append(costs, UserCost{
			UserID:      row.UserID,
			TraceCount:  row.TraceCount,
			TotalCost:   row.TotalCost,
			TotalTokens: row.TotalTokens,
		})
	}
	return costs, nil
}

// TimeSeriesPoint is one bucket of the combined metrics series. Returning all
// metrics together means a dashboard renders every chart from a single query
// instead of one round trip per chart.
type TimeSeriesPoint struct {
	TimeBucket        string  `json:"time_bucket"`
	TraceCount        int64   `json:"trace_count"`
	ErrorCount        int64   `json:"error_count"`
	ErrorRate         float64 `json:"error_rate"`
	TotalCost         float64 `json:"total_cost"`
	InputTokens       int64   `json:"input_tokens"`
	OutputTokens      int64   `json:"output_tokens"`
	TotalTokens       int64   `json:"total_tokens"`
	AvgLatencySeconds float64 `json:"avg_latency_seconds"`
	P95LatencySeconds float64 `json:"p95_latency_seconds"`
}

// TimeSeriesResponse is a bucketed series plus the bucket width used.
type TimeSeriesResponse struct {
	Bucket   string            `json:"bucket"`
	FromTime string            `json:"from_time"`
	ToTime   string            `json:"to_time"`
	Points   []TimeSeriesPoint `json:"points"`
}

// GetMetricsOverTime returns a bucketed series of every dashboard metric.
//
// The bucket width adapts to the range when the caller does not choose one, so
// a one-hour range does not collapse into a single point and a one-year range
// does not return 365 rows.
func (s *EvaluationService) GetMetricsOverTime(ctx context.Context, filter AnalyticsFilter, bucket string) (TimeSeriesResponse, error) {
	from, to := filter.window()
	if bucket == "" {
		bucket = autoBucket(to.Sub(from))
	}
	interval, err := bucketInterval(bucket)
	if err != nil {
		return TimeSeriesResponse{}, err
	}

	rows, err := s.queries.GetTraceMetricsOverTime(ctx, db.GetTraceMetricsOverTimeParams{
		ProjectID:   filter.ProjectID,
		Bucket:      interval,
		Origin:      pgtype.Timestamptz{Time: from, Valid: true},
		FromTime:    pgtype.Timestamptz{Time: from, Valid: true},
		ToTime:      pgtype.Timestamptz{Time: to, Valid: true},
		UserID:      filter.UserID,
		SessionID:   filter.SessionID,
		Environment: filter.Environment,
		Tags:        filter.tags(),
	})
	if err != nil {
		return TimeSeriesResponse{}, fmt.Errorf("getting metrics over time: %w", err)
	}

	points := make([]TimeSeriesPoint, 0, len(rows))
	for _, row := range rows {
		point := TimeSeriesPoint{
			TimeBucket:        row.TimeBucket.Time.UTC().Format(time.RFC3339),
			TraceCount:        row.TraceCount,
			ErrorCount:        row.ErrorCount,
			TotalCost:         row.TotalCost,
			InputTokens:       row.InputTokens,
			OutputTokens:      row.OutputTokens,
			TotalTokens:       row.TotalTokens,
			AvgLatencySeconds: row.AvgLatencySeconds,
			P95LatencySeconds: row.P95LatencySeconds,
		}
		if row.TraceCount > 0 {
			point.ErrorRate = float64(row.ErrorCount) / float64(row.TraceCount)
		}
		points = append(points, point)
	}

	return TimeSeriesResponse{
		Bucket:   bucket,
		FromTime: from.UTC().Format(time.RFC3339),
		ToTime:   to.UTC().Format(time.RFC3339),
		Points:   points,
	}, nil
}

// autoBucket picks a bucket width that yields a readable number of points.
func autoBucket(span time.Duration) string {
	switch {
	case span <= 2*time.Hour:
		return "1 minute"
	case span <= 2*24*time.Hour:
		return "1 hour"
	case span <= 60*24*time.Hour:
		return "1 day"
	case span <= 365*24*time.Hour:
		return "1 week"
	default:
		return "1 month"
	}
}

// allowedBuckets maps each supported bucket name onto a concrete interval.
// Binding a typed interval rather than interpolating the caller's text means
// the bucket parameter has no path into the SQL at all.
var allowedBuckets = map[string]pgtype.Interval{
	"1 minute":   {Microseconds: 60_000_000, Valid: true},
	"5 minutes":  {Microseconds: 5 * 60_000_000, Valid: true},
	"15 minutes": {Microseconds: 15 * 60_000_000, Valid: true},
	"30 minutes": {Microseconds: 30 * 60_000_000, Valid: true},
	"1 hour":     {Microseconds: 3_600_000_000, Valid: true},
	"6 hours":    {Microseconds: 6 * 3_600_000_000, Valid: true},
	"12 hours":   {Microseconds: 12 * 3_600_000_000, Valid: true},
	"1 day":      {Days: 1, Valid: true},
	"1 week":     {Days: 7, Valid: true},
	"1 month":    {Months: 1, Valid: true},
}

// SupportedBuckets lists the bucket widths the analytics API accepts.
func SupportedBuckets() []string {
	names := make([]string, 0, len(allowedBuckets))
	for name := range allowedBuckets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func bucketInterval(bucket string) (pgtype.Interval, error) {
	interval, ok := allowedBuckets[bucket]
	if !ok {
		return pgtype.Interval{}, fmt.Errorf("unsupported bucket %q (supported: %s)",
			bucket, strings.Join(SupportedBuckets(), ", "))
	}
	return interval, nil
}

// DashboardSummary is the full headline set for the analytics landing page.
type DashboardSummary struct {
	Latency    LatencyPercentiles `json:"latency"`
	Cost       CostStats          `json:"cost"`
	TokenUsage TokenUsageStats    `json:"token_usage"`
	ErrorRate  ErrorRateStats     `json:"error_rate"`
	Scores     []ScoreAggregation `json:"scores"`
	Models     []ModelUsage       `json:"models"`

	// AvgScore is the mean across every numeric score in the project, which is
	// the single "is quality holding up" number.
	AvgScore float64 `json:"avg_score"`
}

// GetDashboardSummary assembles the analytics landing page in one call.
func (s *EvaluationService) GetDashboardSummary(ctx context.Context, filter AnalyticsFilter) (DashboardSummary, error) {
	summary := DashboardSummary{}

	latency, err := s.GetLatencyPercentiles(ctx, filter)
	if err != nil {
		return DashboardSummary{}, err
	}
	summary.Latency = latency

	// The remaining aggregates are independent; a failure in one should not
	// blank the whole dashboard, so each falls back to its zero value.
	if cost, err := s.GetCostStats(ctx, filter.ProjectID); err == nil {
		summary.Cost = cost
	}
	if tokens, err := s.GetTokenUsageStats(ctx, filter.ProjectID); err == nil {
		summary.TokenUsage = tokens
	}
	if errors, err := s.GetErrorRateStats(ctx, filter.ProjectID); err == nil {
		summary.ErrorRate = errors
	}
	if models, err := s.GetModelUsage(ctx, filter); err == nil {
		summary.Models = models
	} else {
		summary.Models = []ModelUsage{}
	}

	scores, err := s.GetScoreAggregations(ctx, filter.ProjectID)
	if err != nil {
		scores = []ScoreAggregation{}
	}
	summary.Scores = scores

	var weightedSum float64
	var totalCount int64
	for _, score := range scores {
		weightedSum += score.AvgValue * float64(score.Count)
		totalCount += score.Count
	}
	if totalCount > 0 {
		summary.AvgScore = weightedSum / float64(totalCount)
	}

	return summary, nil
}
