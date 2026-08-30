package services

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/obsevo/obsevo/internal/db"
)

// regressionThreshold is the relative change below which a metric is treated as
// noise rather than a real move. Runs are small, so a tighter threshold would
// flag ordinary variation as a regression.
const regressionThreshold = 0.02

// ExperimentService compares dataset runs against each other.
//
// An experiment in this system is not a separate entity: it is a dataset run
// tagged with the configuration that produced it (prompt version, model), which
// is exactly how Langfuse models it. Comparing two runs of the same dataset is
// therefore comparing two configurations over identical inputs.
type ExperimentService struct {
	queries *db.Queries
}

// NewExperimentService creates a new experiment service.
func NewExperimentService(queries *db.Queries) *ExperimentService {
	return &ExperimentService{queries: queries}
}

// ScoreSummary aggregates one score name across a run.
type ScoreSummary struct {
	Name     string  `json:"name"`
	Count    int64   `json:"count"`
	AvgValue float64 `json:"avg_value"`
	MinValue float64 `json:"min_value"`
	MaxValue float64 `json:"max_value"`
	StdDev   float64 `json:"stddev"`
}

// RunSummary is everything needed to judge a single dataset run.
type RunSummary struct {
	RunID       string `json:"run_id"`
	RunName     string `json:"run_name"`
	DatasetID   string `json:"dataset_id"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at"`

	ItemCount   int64 `json:"item_count"`
	TracedCount int64 `json:"traced_count"`

	TotalCost   float64 `json:"total_cost"`
	AvgCost     float64 `json:"avg_cost"`
	TotalTokens int64   `json:"total_tokens"`

	AvgLatencySeconds float64 `json:"avg_latency_seconds"`
	P50LatencySeconds float64 `json:"p50_latency_seconds"`
	P95LatencySeconds float64 `json:"p95_latency_seconds"`

	Scores []ScoreSummary `json:"scores"`

	// Metadata carries the run's configuration — the prompt version, model and
	// parameters the caller recorded when creating it.
	Metadata any `json:"metadata,omitempty"`
}

// GetRunSummary aggregates one dataset run.
func (s *ExperimentService) GetRunSummary(ctx context.Context, datasetID, runID string) (RunSummary, error) {
	run, err := s.queries.GetDatasetRunByIDAndDataset(ctx, db.GetDatasetRunByIDAndDatasetParams{
		ID:        runID,
		DatasetID: datasetID,
	})
	if err != nil {
		return RunSummary{}, fmt.Errorf("getting dataset run: %w", err)
	}

	stats, err := s.queries.GetDatasetRunStats(ctx, runID)
	if err != nil {
		return RunSummary{}, fmt.Errorf("aggregating dataset run: %w", err)
	}

	scoreRows, err := s.queries.GetDatasetRunScoreStats(ctx, runID)
	if err != nil {
		return RunSummary{}, fmt.Errorf("aggregating run scores: %w", err)
	}

	scores := make([]ScoreSummary, 0, len(scoreRows))
	for _, row := range scoreRows {
		scores = append(scores, ScoreSummary{
			Name:     row.Name,
			Count:    row.Count,
			AvgValue: row.AvgValue,
			MinValue: row.MinValue,
			MaxValue: row.MaxValue,
			StdDev:   row.StddevValue,
		})
	}

	summary := RunSummary{
		RunID:             run.ID,
		RunName:           run.Name,
		DatasetID:         run.DatasetID,
		Description:       run.Description.String,
		CreatedAt:         run.CreatedAt.Time.Format(timeFormatRFC3339),
		ItemCount:         stats.ItemCount,
		TracedCount:       stats.TracedCount,
		TotalCost:         stats.TotalCost,
		AvgCost:           stats.AvgCost,
		TotalTokens:       stats.TotalTokens,
		AvgLatencySeconds: stats.AvgLatencySeconds,
		P50LatencySeconds: stats.P50LatencySeconds,
		P95LatencySeconds: stats.P95LatencySeconds,
		Scores:            scores,
	}
	if len(run.Metadata) > 0 {
		summary.Metadata = run.Metadata
	}

	return summary, nil
}

// ListRunSummaries aggregates every run of a dataset, newest first.
func (s *ExperimentService) ListRunSummaries(ctx context.Context, datasetID string) ([]RunSummary, error) {
	runs, err := s.queries.GetDatasetRunsByDatasetID(ctx, datasetID)
	if err != nil {
		return nil, fmt.Errorf("listing dataset runs: %w", err)
	}

	summaries := make([]RunSummary, 0, len(runs))
	for _, run := range runs {
		summary, err := s.GetRunSummary(ctx, datasetID, run.ID)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// MetricDelta describes how one metric moved between two runs.
type MetricDelta struct {
	Metric   string  `json:"metric"`
	Baseline float64 `json:"baseline"`
	Current  float64 `json:"current"`
	Absolute float64 `json:"absolute_change"`

	// Relative is the fractional change from the baseline. It is null-safe:
	// when the baseline is zero the field is reported as 0 and Comparable is
	// false, because a percentage against zero is meaningless.
	Relative   float64 `json:"relative_change"`
	Comparable bool    `json:"comparable"`

	// HigherIsBetter records the metric's direction so the caller does not have
	// to know that lower cost is good but a lower score is not.
	HigherIsBetter bool `json:"higher_is_better"`

	// Verdict is "improved", "regressed" or "unchanged".
	Verdict string `json:"verdict"`
}

// ComparisonResult is the full baseline-versus-candidate report.
type ComparisonResult struct {
	Baseline RunSummary `json:"baseline"`
	Current  RunSummary `json:"current"`

	Quality []MetricDelta `json:"quality"`
	Cost    []MetricDelta `json:"cost"`
	Latency []MetricDelta `json:"latency"`

	// Regressions lists every metric that moved in the wrong direction by more
	// than the noise threshold.
	Regressions []MetricDelta `json:"regressions"`

	// Summary is a one-line, human-readable verdict.
	Summary string `json:"summary"`
}

// CompareRuns reports how a candidate run differs from a baseline run.
func (s *ExperimentService) CompareRuns(ctx context.Context, datasetID, baselineRunID, currentRunID string) (ComparisonResult, error) {
	baseline, err := s.GetRunSummary(ctx, datasetID, baselineRunID)
	if err != nil {
		return ComparisonResult{}, fmt.Errorf("loading baseline run: %w", err)
	}
	current, err := s.GetRunSummary(ctx, datasetID, currentRunID)
	if err != nil {
		return ComparisonResult{}, fmt.Errorf("loading current run: %w", err)
	}

	return CompareSummaries(baseline, current), nil
}

// CompareSummaries computes the comparison between two already-aggregated runs.
// It is separated from CompareRuns so the comparison rules can be exercised
// without a database.
func CompareSummaries(baseline, current RunSummary) ComparisonResult {
	result := ComparisonResult{
		Baseline:    baseline,
		Current:     current,
		Quality:     []MetricDelta{},
		Regressions: []MetricDelta{},
	}

	// Quality: every score present in either run, matched by name.
	for _, name := range unionScoreNames(baseline.Scores, current.Scores) {
		result.Quality = append(result.Quality, newDelta(
			"score."+name,
			scoreValue(baseline.Scores, name),
			scoreValue(current.Scores, name),
			true,
		))
	}

	result.Cost = []MetricDelta{
		newDelta("total_cost", baseline.TotalCost, current.TotalCost, false),
		newDelta("avg_cost", baseline.AvgCost, current.AvgCost, false),
		newDelta("total_tokens", float64(baseline.TotalTokens), float64(current.TotalTokens), false),
	}

	result.Latency = []MetricDelta{
		newDelta("avg_latency_seconds", baseline.AvgLatencySeconds, current.AvgLatencySeconds, false),
		newDelta("p50_latency_seconds", baseline.P50LatencySeconds, current.P50LatencySeconds, false),
		newDelta("p95_latency_seconds", baseline.P95LatencySeconds, current.P95LatencySeconds, false),
	}

	for _, group := range [][]MetricDelta{result.Quality, result.Cost, result.Latency} {
		for _, delta := range group {
			if delta.Verdict == "regressed" {
				result.Regressions = append(result.Regressions, delta)
			}
		}
	}

	result.Summary = summarize(result)
	return result
}

// newDelta computes one metric's movement and verdict.
func newDelta(metric string, baseline, current float64, higherIsBetter bool) MetricDelta {
	delta := MetricDelta{
		Metric:         metric,
		Baseline:       baseline,
		Current:        current,
		Absolute:       current - baseline,
		HigherIsBetter: higherIsBetter,
		Verdict:        "unchanged",
	}

	if baseline != 0 {
		delta.Relative = (current - baseline) / math.Abs(baseline)
		delta.Comparable = true
	}

	// Without a baseline to divide by, fall back to the absolute move so a
	// metric going from 0 to something is still reported.
	change := delta.Relative
	if !delta.Comparable {
		if delta.Absolute == 0 {
			return delta
		}
		change = math.Copysign(1, delta.Absolute)
	}

	if math.Abs(change) < regressionThreshold {
		return delta
	}

	improved := change > 0
	if !higherIsBetter {
		improved = change < 0
	}
	if improved {
		delta.Verdict = "improved"
	} else {
		delta.Verdict = "regressed"
	}

	return delta
}

// summarize renders the headline movements as one sentence.
func summarize(result ComparisonResult) string {
	var parts []string

	for _, delta := range result.Quality {
		if delta.Verdict != "unchanged" && delta.Comparable {
			parts = append(parts, fmt.Sprintf("%s %+.1f%% quality", delta.Metric, delta.Relative*100))
		}
	}
	for _, delta := range result.Cost {
		if delta.Metric == "total_cost" && delta.Verdict != "unchanged" && delta.Comparable {
			parts = append(parts, fmt.Sprintf("%+.1f%% cost", delta.Relative*100))
		}
	}
	for _, delta := range result.Latency {
		if delta.Metric == "p95_latency_seconds" && delta.Verdict != "unchanged" && delta.Comparable {
			parts = append(parts, fmt.Sprintf("%+.1f%% p95 latency", delta.Relative*100))
		}
	}

	if len(parts) == 0 {
		return "no significant change"
	}

	summary := parts[0]
	for _, part := range parts[1:] {
		summary += ", " + part
	}
	if len(result.Regressions) > 0 {
		summary += fmt.Sprintf(" (%d regression(s))", len(result.Regressions))
	}
	return summary
}

// unionScoreNames lists every score name appearing in either run, sorted.
func unionScoreNames(left, right []ScoreSummary) []string {
	seen := make(map[string]struct{}, len(left)+len(right))
	for _, s := range left {
		seen[s.Name] = struct{}{}
	}
	for _, s := range right {
		seen[s.Name] = struct{}{}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// scoreValue returns a named score's average, or zero when the run lacks it.
func scoreValue(scores []ScoreSummary, name string) float64 {
	for _, s := range scores {
		if s.Name == name {
			return s.AvgValue
		}
	}
	return 0
}

// timeFormatRFC3339 is the timestamp layout used in API responses.
const timeFormatRFC3339 = "2006-01-02T15:04:05Z07:00"
