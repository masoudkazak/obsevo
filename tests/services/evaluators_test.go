package services_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/obsevo/obsevo/internal/db"
	"github.com/obsevo/obsevo/internal/services"
)

func TestLengthCheckEvaluator_InputTooShort(t *testing.T) {
	config := `{"evaluator":"length_check","min_length":10,"field":"input"}`
	evaluator, err := buildTestEvaluator("CODE", config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trace := db.Trace{
		Input: json.RawMessage(`"hi"`),
	}
	score, reason, err := evaluator.Evaluate(context.Background(), trace, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != 0 {
		t.Errorf("expected score 0, got %f", score)
	}
	if reason == "" {
		t.Error("expected non-empty reason")
	}
}

func TestLengthCheckEvaluator_InputOk(t *testing.T) {
	config := `{"evaluator":"length_check","min_length":2,"field":"input"}`
	evaluator, err := buildTestEvaluator("CODE", config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trace := db.Trace{
		Input: json.RawMessage(`"hello world"`),
	}
	score, _, err := evaluator.Evaluate(context.Background(), trace, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != 1 {
		t.Errorf("expected score 1, got %f", score)
	}
}

func TestLengthCheckEvaluator_InputTooLong(t *testing.T) {
	config := `{"evaluator":"length_check","max_length":5,"field":"input"}`
	evaluator, err := buildTestEvaluator("CODE", config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trace := db.Trace{
		Input: json.RawMessage(`"this is a long string"`),
	}
	score, _, err := evaluator.Evaluate(context.Background(), trace, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != 0 {
		t.Errorf("expected score 0, got %f", score)
	}
}

func TestKeywordCheckEvaluator_Contains(t *testing.T) {
	config := `{"evaluator":"keyword_check","keywords":["hello","world"],"field":"input","mode":"contains"}`
	evaluator, err := buildTestEvaluator("CODE", config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trace := db.Trace{
		Input: json.RawMessage(`"hello there, world"`),
	}
	score, _, err := evaluator.Evaluate(context.Background(), trace, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != 1.0 {
		t.Errorf("expected score 1.0, got %f", score)
	}
}

func TestKeywordCheckEvaluator_NotContains(t *testing.T) {
	config := `{"evaluator":"keyword_check","keywords":["forbidden"],"field":"input","mode":"not_contains"}`
	evaluator, err := buildTestEvaluator("CODE", config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trace := db.Trace{
		Input: json.RawMessage(`"this is clean"`),
	}
	score, _, err := evaluator.Evaluate(context.Background(), trace, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != 1.0 {
		t.Errorf("expected score 1.0 (no forbidden words), got %f", score)
	}
}

func TestRegexCheckEvaluator_Match(t *testing.T) {
	config := `{"evaluator":"regex_check","pattern":"hello","field":"input"}`
	evaluator, err := buildTestEvaluator("CODE", config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trace := db.Trace{
		Input: json.RawMessage(`"hello"`),
	}
	score, _, err := evaluator.Evaluate(context.Background(), trace, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != 1.0 {
		t.Errorf("expected score 1.0, got %f", score)
	}
}

func TestRegexCheckEvaluator_NoMatch(t *testing.T) {
	config := `{"evaluator":"regex_check","pattern":"xyz","field":"input"}`
	evaluator, err := buildTestEvaluator("CODE", config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trace := db.Trace{
		Input: json.RawMessage(`"hello"`),
	}
	score, _, err := evaluator.Evaluate(context.Background(), trace, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != 0 {
		t.Errorf("expected score 0, got %f", score)
	}
}

func TestNumericRangeEvaluator_InRange(t *testing.T) {
	config := `{"evaluator":"numeric_range","min":0.0,"max":1.0,"field":"cost"}`
	evaluator, err := buildTestEvaluator("CODE", config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trace := db.Trace{
		TotalCost: pgtypeFloat8(0.5),
	}
	score, _, err := evaluator.Evaluate(context.Background(), trace, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != 1.0 {
		t.Errorf("expected score 1.0, got %f", score)
	}
}

func TestNumericRangeEvaluator_OutOfRange(t *testing.T) {
	config := `{"evaluator":"numeric_range","min":0.0,"max":1.0,"field":"cost"}`
	evaluator, err := buildTestEvaluator("CODE", config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trace := db.Trace{
		TotalCost: pgtypeFloat8(5.0),
	}
	score, _, err := evaluator.Evaluate(context.Background(), trace, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != 0 {
		t.Errorf("expected score 0, got %f", score)
	}
}

func TestNoopEvaluator(t *testing.T) {
	config := `{"evaluator":"unknown_type"}`
	evaluator, err := buildTestEvaluator("CODE", config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	score, reason, err := evaluator.Evaluate(context.Background(), db.Trace{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != 0 {
		t.Errorf("expected score 0, got %f", score)
	}
	if reason != "noop evaluator" {
		t.Errorf("expected 'noop evaluator', got %q", reason)
	}
}

func buildTestEvaluator(evalType string, configJSON string) (services.Evaluator, error) {
	config := json.RawMessage(configJSON)
	return services.BuildEvaluatorForTest(evalType, config)
}

func pgtypeFloat8(v float64) pgtype.Float8 {
	return pgtype.Float8{Float64: v, Valid: true}
}

func pgtypeText(v string) pgtype.Text {
	return pgtype.Text{String: v, Valid: v != ""}
}

// timestamptz parses an RFC 3339 literal for use in a test fixture.
func timestamptz(value string) pgtype.Timestamptz {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return pgtype.Timestamptz{Time: parsed, Valid: true}
}
