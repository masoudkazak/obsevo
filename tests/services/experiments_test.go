package services_test

import (
	"testing"

	"github.com/langfuse-light/langfuse-light/internal/services"
)

// findDelta locates a metric in a comparison group.
func findDelta(t *testing.T, deltas []services.MetricDelta, metric string) services.MetricDelta {
	t.Helper()
	for _, delta := range deltas {
		if delta.Metric == metric {
			return delta
		}
	}
	t.Fatalf("metric %q not found", metric)
	return services.MetricDelta{}
}

// The scenario from the project brief: a candidate model that is better, cheaper
// and slower. The comparison has to report each of those independently and in
// the right direction.
func TestCompareSummariesReportsQualityCostAndLatency(t *testing.T) {
	baseline := services.RunSummary{
		RunName:           "baseline",
		ItemCount:         3,
		TotalCost:         0.0060,
		AvgCost:           0.0020,
		TotalTokens:       450,
		AvgLatencySeconds: 1.0,
		P50LatencySeconds: 1.0,
		P95LatencySeconds: 1.0,
		Scores:            []services.ScoreSummary{{Name: "correctness", Count: 3, AvgValue: 0.70}},
	}
	current := services.RunSummary{
		RunName:           "candidate",
		ItemCount:         3,
		TotalCost:         0.0048,
		AvgCost:           0.0016,
		TotalTokens:       480,
		AvgLatencySeconds: 1.1,
		P50LatencySeconds: 1.1,
		P95LatencySeconds: 1.1,
		Scores:            []services.ScoreSummary{{Name: "correctness", Count: 3, AvgValue: 0.756}},
	}

	result := services.CompareSummaries(baseline, current)

	quality := findDelta(t, result.Quality, "score.correctness")
	if quality.Verdict != "improved" {
		t.Errorf("expected quality to improve, got %q", quality.Verdict)
	}
	if delta := quality.Relative*100 - 8.0; delta > 0.01 || delta < -0.01 {
		t.Errorf("expected +8%% quality, got %+.2f%%", quality.Relative*100)
	}

	cost := findDelta(t, result.Cost, "total_cost")
	if cost.Verdict != "improved" {
		t.Errorf("expected lower cost to count as an improvement, got %q", cost.Verdict)
	}
	if delta := cost.Relative*100 + 20.0; delta > 0.01 || delta < -0.01 {
		t.Errorf("expected -20%% cost, got %+.2f%%", cost.Relative*100)
	}

	latency := findDelta(t, result.Latency, "p95_latency_seconds")
	if latency.Verdict != "regressed" {
		t.Errorf("expected higher latency to count as a regression, got %q", latency.Verdict)
	}
	if delta := latency.Relative*100 - 10.0; delta > 0.01 || delta < -0.01 {
		t.Errorf("expected +10%% latency, got %+.2f%%", latency.Relative*100)
	}

	if len(result.Regressions) == 0 {
		t.Error("expected the latency regression to be listed")
	}
	if result.Summary == "" || result.Summary == "no significant change" {
		t.Errorf("expected a descriptive summary, got %q", result.Summary)
	}
}

func TestCompareSummariesTreatsSmallMovesAsNoise(t *testing.T) {
	baseline := services.RunSummary{
		TotalCost:         1.0,
		AvgLatencySeconds: 1.0,
		P50LatencySeconds: 1.0,
		P95LatencySeconds: 1.0,
		Scores:            []services.ScoreSummary{{Name: "quality", AvgValue: 0.800}},
	}
	current := services.RunSummary{
		// Every move here is under the 2% noise threshold.
		TotalCost:         1.005,
		AvgLatencySeconds: 1.005,
		P50LatencySeconds: 1.005,
		P95LatencySeconds: 1.005,
		Scores:            []services.ScoreSummary{{Name: "quality", AvgValue: 0.805}},
	}

	result := services.CompareSummaries(baseline, current)

	if len(result.Regressions) != 0 {
		t.Errorf("expected no regressions from sub-threshold movement, got %v", result.Regressions)
	}
	if result.Summary != "no significant change" {
		t.Errorf("expected the summary to report no change, got %q", result.Summary)
	}
}

func TestCompareSummariesHandlesAZeroBaseline(t *testing.T) {
	baseline := services.RunSummary{Scores: []services.ScoreSummary{{Name: "quality", AvgValue: 0}}}
	current := services.RunSummary{Scores: []services.ScoreSummary{{Name: "quality", AvgValue: 0.5}}}

	result := services.CompareSummaries(baseline, current)
	quality := findDelta(t, result.Quality, "score.quality")

	// A percentage against zero is meaningless, so the comparison must say so
	// rather than report an infinite improvement.
	if quality.Comparable {
		t.Error("expected a zero baseline to be marked not comparable")
	}
	if quality.Verdict != "improved" {
		t.Errorf("expected the absolute move to still register, got %q", quality.Verdict)
	}
	if quality.Absolute != 0.5 {
		t.Errorf("expected an absolute change of 0.5, got %v", quality.Absolute)
	}
}

func TestCompareSummariesIncludesScoresPresentInOnlyOneRun(t *testing.T) {
	baseline := services.RunSummary{Scores: []services.ScoreSummary{{Name: "only_baseline", AvgValue: 0.9}}}
	current := services.RunSummary{Scores: []services.ScoreSummary{{Name: "only_current", AvgValue: 0.4}}}

	result := services.CompareSummaries(baseline, current)

	if len(result.Quality) != 2 {
		t.Fatalf("expected both score names to appear, got %d", len(result.Quality))
	}
	// A score that vanished from the candidate reads as quality dropping to zero,
	// which is the correct alarm: the metric is no longer being produced.
	dropped := findDelta(t, result.Quality, "score.only_baseline")
	if dropped.Verdict != "regressed" {
		t.Errorf("expected a missing score to register as a regression, got %q", dropped.Verdict)
	}
}

func TestCompareSummariesMarksDirectionPerMetric(t *testing.T) {
	result := services.CompareSummaries(services.RunSummary{}, services.RunSummary{})

	for _, delta := range result.Cost {
		if delta.HigherIsBetter {
			t.Errorf("cost metric %q should not be higher-is-better", delta.Metric)
		}
	}
	for _, delta := range result.Latency {
		if delta.HigherIsBetter {
			t.Errorf("latency metric %q should not be higher-is-better", delta.Metric)
		}
	}
}
