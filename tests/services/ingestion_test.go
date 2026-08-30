package services_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/langfuse-light/langfuse-light/internal/db"
	"github.com/langfuse-light/langfuse-light/internal/services"
)

// Every Langfuse SDK stamps events with RFC 3339. A regression here silently
// nulls start_time, which the NOT NULL column then rejects — that is the defect
// this test exists to prevent recurring.
func TestParseIngestionTimeAcceptsSDKTimestamps(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"RFC3339 UTC", "2026-08-30T10:00:00Z", "2026-08-30T10:00:00Z"},
		{"RFC3339 with milliseconds", "2026-08-30T10:00:00.123Z", "2026-08-30T10:00:00.123Z"},
		{"RFC3339 with microseconds", "2026-08-30T10:00:00.123456Z", "2026-08-30T10:00:00.123456Z"},
		{"RFC3339 with offset", "2026-08-30T12:30:00+02:30", "2026-08-30T10:00:00Z"},
		{"naive datetime", "2026-08-30T10:00:00", "2026-08-30T10:00:00Z"},
		{"postgres wire format", "2026-08-30 10:00:00+00", "2026-08-30T10:00:00Z"},
		{"date only", "2026-08-30", "2026-08-30T00:00:00Z"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, ok := services.ParseIngestionTime(tc.input)
			if !ok {
				t.Fatalf("expected %q to parse", tc.input)
			}
			if got := parsed.Format(time.RFC3339Nano); got != tc.want {
				t.Errorf("expected %s, got %s", tc.want, got)
			}
			if parsed.Location() != time.UTC {
				t.Errorf("expected the result in UTC, got %s", parsed.Location())
			}
		})
	}
}

func TestParseIngestionTimeRejectsGarbage(t *testing.T) {
	for _, input := range []string{"", "not a time", "30/08/2026", "1788071446"} {
		if _, ok := services.ParseIngestionTime(input); ok {
			t.Errorf("expected %q to be rejected", input)
		}
	}
}

// The server accepts every token-usage shape the SDKs and the providers they
// wrap emit, and stores exactly one.
func TestNormalizeUsageAcceptsEveryKnownShape(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  services.TokenUsage
	}{
		{
			"Langfuse short form",
			`{"input":100,"output":50}`,
			services.TokenUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150},
		},
		{
			"OpenAI form",
			`{"promptTokens":100,"completionTokens":50,"totalTokens":150}`,
			services.TokenUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150},
		},
		{
			"snake_case form",
			`{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}`,
			services.TokenUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150},
		},
		{
			"stored form",
			`{"input_tokens":100,"output_tokens":50,"total_tokens":150}`,
			services.TokenUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150},
		},
		{
			"total derived when absent",
			`{"input":7,"output":3}`,
			services.TokenUsage{InputTokens: 7, OutputTokens: 3, TotalTokens: 10},
		},
		{
			"total only",
			`{"total":42}`,
			services.TokenUsage{TotalTokens: 42},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, usage := services.NormalizeUsage(json.RawMessage(tc.input))
			if usage != tc.want {
				t.Errorf("expected %+v, got %+v", tc.want, usage)
			}

			// The normalized JSON is what analytics queries read, so it must
			// always use the stored field names.
			var stored map[string]int64
			if err := json.Unmarshal(encoded, &stored); err != nil {
				t.Fatalf("normalized usage is not valid JSON: %v", err)
			}
			for _, key := range []string{"input_tokens", "output_tokens", "total_tokens"} {
				if _, ok := stored[key]; !ok {
					t.Errorf("expected the normalized payload to contain %q, got %v", key, stored)
				}
			}
		})
	}
}

func TestNormalizeUsageIgnoresUnusableInput(t *testing.T) {
	for _, input := range []string{"", "null", "not json", `{}`, `{"unrelated":1}`} {
		encoded, usage := services.NormalizeUsage(json.RawMessage(input))
		if encoded != nil {
			t.Errorf("expected no normalized payload for %q, got %s", input, encoded)
		}
		if usage != (services.TokenUsage{}) {
			t.Errorf("expected zero usage for %q, got %+v", input, usage)
		}
	}
}

func TestBuildTargetDecodesTraceContent(t *testing.T) {
	trace := db.Trace{
		Input:      json.RawMessage(`"what is langfuse?"`),
		Output:     json.RawMessage(`"an observability platform"`),
		Metadata:   json.RawMessage(`{"tenant":"acme","retries":2}`),
		TokenUsage: json.RawMessage(`{"input_tokens":100,"output_tokens":50,"total_tokens":150}`),
		TotalCost:  pgtypeFloat8(0.25),
		StartTime:  timestamptz("2026-08-30T10:00:00Z"),
		EndTime:    timestamptz("2026-08-30T10:00:02Z"),
	}

	observations := []db.Observation{
		{Status: "OK", Level: "DEFAULT", Model: pgtypeText("gpt-4o")},
		{Status: "ERROR", Level: "ERROR"},
		{Status: "DEFAULT", Level: "DEFAULT"},
	}

	target := services.BuildTarget(trace, observations, "expected answer")

	// A JSON string is unwrapped so evaluators see the text, not its quoted form.
	if target.Input != "what is langfuse?" {
		t.Errorf("expected the decoded input, got %q", target.Input)
	}
	if target.Output != "an observability platform" {
		t.Errorf("expected the decoded output, got %q", target.Output)
	}
	if target.Expected != "expected answer" {
		t.Errorf("expected the dataset expectation to be carried through, got %q", target.Expected)
	}
	if target.Cost != 0.25 {
		t.Errorf("expected cost 0.25, got %v", target.Cost)
	}
	if target.TotalTokens != 150 || target.InputTokens != 100 || target.OutputTokens != 50 {
		t.Errorf("expected usage 100/50/150, got %d/%d/%d",
			target.InputTokens, target.OutputTokens, target.TotalTokens)
	}
	if target.LatencySeconds != 2 {
		t.Errorf("expected 2s latency, got %v", target.LatencySeconds)
	}
	if target.ObservationCount != 3 {
		t.Errorf("expected 3 observations, got %d", target.ObservationCount)
	}
	if target.ErrorCount != 1 {
		t.Errorf("expected 1 error observation, got %d", target.ErrorCount)
	}
	if target.Model != "gpt-4o" {
		t.Errorf("expected the model from the generation, got %q", target.Model)
	}
	if target.Metadata["tenant"] != "acme" {
		t.Errorf("expected decoded metadata, got %v", target.Metadata)
	}
}

func TestBuildTargetKeepsNonStringJSONAsJSON(t *testing.T) {
	trace := db.Trace{Input: json.RawMessage(`{"question":"why?"}`)}

	target := services.BuildTarget(trace, nil, "")
	if target.Input != `{"question":"why?"}` {
		t.Errorf("expected a JSON object to be preserved verbatim, got %q", target.Input)
	}
}
