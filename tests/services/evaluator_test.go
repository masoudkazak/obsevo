package services_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/langfuse-light/langfuse-light/internal/services/evaluator"
)

// build constructs an evaluator or fails the test.
func build(t *testing.T, name, config string) evaluator.Evaluator {
	t.Helper()
	impl, err := evaluator.Build(name, json.RawMessage(config), evaluator.Deps{})
	if err != nil {
		t.Fatalf("building %s: %v", name, err)
	}
	return impl
}

// run evaluates a target or fails the test.
func run(t *testing.T, impl evaluator.Evaluator, target evaluator.Target) evaluator.Result {
	t.Helper()
	result, err := impl.Evaluate(context.Background(), target)
	if err != nil {
		t.Fatalf("evaluating: %v", err)
	}
	return result
}

func TestRegistryCoversEveryRequiredEvaluator(t *testing.T) {
	// The project brief names these ten evaluator capabilities; the registry is
	// the single place they are declared, so this guards against one being
	// dropped in a refactor.
	required := []string{
		"exact_match",
		"keyword",
		"regex",
		"numeric_range",
		"json_valid",
		"json_schema",
		"custom_code",
		"llm_judge",
		"semantic_similarity",
		"embedding_similarity",
	}

	for _, name := range required {
		if !evaluator.Exists(name) {
			t.Errorf("evaluator %q is not registered", name)
		}
	}

	for _, descriptor := range evaluator.Descriptors() {
		if descriptor.Description == "" {
			t.Errorf("evaluator %q has no description", descriptor.Name)
		}
	}
}

func TestBuildRejectsUnknownEvaluator(t *testing.T) {
	if _, err := evaluator.Build("does_not_exist", json.RawMessage(`{}`), evaluator.Deps{}); err == nil {
		t.Fatal("expected an error for an unknown evaluator")
	}
}

func TestExactMatch(t *testing.T) {
	impl := build(t, "exact_match", `{"field":"output","trim_space":true}`)

	if got := run(t, impl, evaluator.Target{Output: " hello ", Expected: "hello"}); got.Score != 1 {
		t.Errorf("expected 1 for a trimmed match, got %v (%s)", got.Score, got.Reason)
	}
	if got := run(t, impl, evaluator.Target{Output: "hello", Expected: "goodbye"}); got.Score != 0 {
		t.Errorf("expected 0 for a mismatch, got %v", got.Score)
	}
}

func TestExactMatchIsCaseInsensitiveByDefault(t *testing.T) {
	impl := build(t, "exact_match", `{}`)
	if got := run(t, impl, evaluator.Target{Output: "HELLO", Expected: "hello"}); got.Score != 1 {
		t.Errorf("expected a case-insensitive match, got %v", got.Score)
	}

	sensitive := build(t, "exact_match", `{"case_sensitive":true}`)
	if got := run(t, sensitive, evaluator.Target{Output: "HELLO", Expected: "hello"}); got.Score != 0 {
		t.Errorf("expected case sensitivity to reject, got %v", got.Score)
	}
}

func TestExactMatchRequiresAnExpectedValue(t *testing.T) {
	impl := build(t, "exact_match", `{}`)
	if _, err := impl.Evaluate(context.Background(), evaluator.Target{Output: "x"}); err == nil {
		t.Fatal("expected an error when no expected value is available")
	}
}

func TestKeywordScoresPartialMatches(t *testing.T) {
	impl := build(t, "keyword", `{"keywords":["alpha","beta","gamma"],"field":"output"}`)

	result := run(t, impl, evaluator.Target{Output: "alpha and gamma"})
	if result.Score < 0.66 || result.Score > 0.67 {
		t.Errorf("expected 2/3, got %v", result.Score)
	}
	if !strings.Contains(result.Reason, "beta") {
		t.Errorf("expected the reason to name the missing keyword, got %q", result.Reason)
	}
}

func TestKeywordNotContainsInverts(t *testing.T) {
	impl := build(t, "keyword", `{"keywords":["forbidden"],"mode":"not_contains","field":"output"}`)

	if got := run(t, impl, evaluator.Target{Output: "all clean"}); got.Score != 1 {
		t.Errorf("expected 1 when the forbidden word is absent, got %v", got.Score)
	}
	if got := run(t, impl, evaluator.Target{Output: "forbidden content"}); got.Score != 0 {
		t.Errorf("expected 0 when the forbidden word is present, got %v", got.Score)
	}
}

func TestRegexNegate(t *testing.T) {
	impl := build(t, "regex", `{"pattern":"^\\d+$","field":"output","negate":true}`)

	if got := run(t, impl, evaluator.Target{Output: "abc"}); got.Score != 1 {
		t.Errorf("expected 1 when the pattern does not match, got %v", got.Score)
	}
	if got := run(t, impl, evaluator.Target{Output: "123"}); got.Score != 0 {
		t.Errorf("expected 0 when the pattern matches, got %v", got.Score)
	}
}

func TestRegexRejectsAnInvalidPattern(t *testing.T) {
	if _, err := evaluator.Build("regex", json.RawMessage(`{"pattern":"["}`), evaluator.Deps{}); err == nil {
		t.Fatal("expected an invalid pattern to be rejected at configuration time")
	}
}

func TestNumericRangeSupportsEveryDocumentedField(t *testing.T) {
	target := evaluator.Target{
		Cost:             0.5,
		TotalTokens:      1500,
		InputTokens:      1000,
		OutputTokens:     500,
		LatencySeconds:   2.5,
		ObservationCount: 4,
		ErrorCount:       1,
		Output:           "abcd",
	}

	cases := []struct {
		field string
		cfg   string
		want  float64
	}{
		{"cost", `{"field":"cost","min":0,"max":1}`, 1},
		{"cost out of range", `{"field":"cost","min":0,"max":0.1}`, 0},
		{"tokens", `{"field":"tokens","max":2000}`, 1},
		{"input_tokens", `{"field":"input_tokens","min":900}`, 1},
		{"output_tokens", `{"field":"output_tokens","max":100}`, 0},
		{"latency", `{"field":"latency","max":3}`, 1},
		{"observation_count", `{"field":"observation_count","min":4}`, 1},
		{"error_count", `{"field":"error_count","max":0}`, 0},
		{"output_length", `{"field":"output_length","min":4,"max":4}`, 1},
	}

	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			impl := build(t, "numeric_range", tc.cfg)
			if got := run(t, impl, target); got.Score != tc.want {
				t.Errorf("expected %v, got %v (%s)", tc.want, got.Score, got.Reason)
			}
		})
	}
}

func TestNumericRangeRejectsAnUnknownField(t *testing.T) {
	impl := build(t, "numeric_range", `{"field":"nonsense","min":0}`)
	if _, err := impl.Evaluate(context.Background(), evaluator.Target{}); err == nil {
		t.Fatal("expected an error for an unsupported field")
	}
}

func TestJSONValidHandlesCodeFences(t *testing.T) {
	impl := build(t, "json_valid", `{"field":"output"}`)

	fenced := "```json\n{\"ok\": true}\n```"
	if got := run(t, impl, evaluator.Target{Output: fenced}); got.Score != 1 {
		t.Errorf("expected a fenced JSON block to be accepted, got %v (%s)", got.Score, got.Reason)
	}
	if got := run(t, impl, evaluator.Target{Output: "not json"}); got.Score != 0 {
		t.Errorf("expected 0 for non-JSON, got %v", got.Score)
	}
	if got := run(t, impl, evaluator.Target{Output: ""}); got.Score != 0 {
		t.Errorf("expected 0 for an empty field, got %v", got.Score)
	}
}

func TestJSONSchemaValidates(t *testing.T) {
	schema := `{
		"field": "output",
		"schema": {
			"type": "object",
			"required": ["name", "age"],
			"properties": {
				"name": {"type": "string", "minLength": 2},
				"age": {"type": "integer", "minimum": 0, "maximum": 130},
				"tags": {"type": "array", "items": {"type": "string"}, "maxItems": 3}
			},
			"additionalProperties": false
		}
	}`
	impl := build(t, "json_schema", schema)

	valid := `{"name":"Ada","age":36,"tags":["x","y"]}`
	if got := run(t, impl, evaluator.Target{Output: valid}); got.Score != 1 {
		t.Errorf("expected a valid document to pass, got %v (%s)", got.Score, got.Reason)
	}

	cases := map[string]string{
		"missing required":    `{"name":"Ada"}`,
		"wrong type":          `{"name":"Ada","age":"36"}`,
		"below minLength":     `{"name":"A","age":36}`,
		"above maximum":       `{"name":"Ada","age":200}`,
		"too many items":      `{"name":"Ada","age":36,"tags":["a","b","c","d"]}`,
		"additional property": `{"name":"Ada","age":36,"extra":1}`,
		"not an object":       `["Ada"]`,
	}
	for label, document := range cases {
		t.Run(label, func(t *testing.T) {
			result := run(t, impl, evaluator.Target{Output: document})
			if result.Score != 0 {
				t.Errorf("expected %s to fail, got %v (%s)", label, result.Score, result.Reason)
			}
			if result.Reason == "" {
				t.Error("expected a reason explaining the violation")
			}
		})
	}
}

func TestJSONSchemaEnumAndAnyOf(t *testing.T) {
	impl := build(t, "json_schema", `{
		"schema": {
			"type": "object",
			"properties": {
				"status": {"enum": ["ok", "error"]},
				"value": {"anyOf": [{"type": "string"}, {"type": "number"}]}
			},
			"required": ["status"]
		}
	}`)

	if got := run(t, impl, evaluator.Target{Output: `{"status":"ok","value":1}`}); got.Score != 1 {
		t.Errorf("expected a valid enum/anyOf document to pass, got %v (%s)", got.Score, got.Reason)
	}
	if got := run(t, impl, evaluator.Target{Output: `{"status":"maybe"}`}); got.Score != 0 {
		t.Errorf("expected an out-of-enum value to fail, got %v", got.Score)
	}
	if got := run(t, impl, evaluator.Target{Output: `{"status":"ok","value":true}`}); got.Score != 0 {
		t.Errorf("expected a value matching no anyOf branch to fail, got %v", got.Score)
	}
}

func TestJSONSchemaPartialCredit(t *testing.T) {
	impl := build(t, "json_schema", `{
		"partial_credit": true,
		"schema": {"type":"object","required":["a","b","c","d"]}
	}`)

	result := run(t, impl, evaluator.Target{Output: `{"a":1,"b":2}`})
	if result.Score <= 0 || result.Score >= 1 {
		t.Errorf("expected partial credit strictly between 0 and 1, got %v", result.Score)
	}
}

func TestJSONSchemaRejectsAnInvalidSchema(t *testing.T) {
	if _, err := evaluator.Build("json_schema", json.RawMessage(`{"schema":{"pattern":"["}}`), evaluator.Deps{}); err == nil {
		t.Fatal("expected an invalid schema to be rejected at configuration time")
	}
}

func TestCustomCodeArithmeticAndComparison(t *testing.T) {
	impl := build(t, "custom_code", `{"expression":"cost < 0.01 && tokens > 100"}`)

	if got := run(t, impl, evaluator.Target{Cost: 0.005, TotalTokens: 500}); got.Score != 1 {
		t.Errorf("expected the condition to hold, got %v (%s)", got.Score, got.Reason)
	}
	if got := run(t, impl, evaluator.Target{Cost: 0.05, TotalTokens: 500}); got.Score != 0 {
		t.Errorf("expected the condition to fail, got %v", got.Score)
	}
}

func TestCustomCodeFunctions(t *testing.T) {
	cases := []struct {
		expression string
		target     evaluator.Target
		want       float64
	}{
		{`len(output)`, evaluator.Target{Output: "abcde"}, 5},
		{`contains(lower(output), "hello")`, evaluator.Target{Output: "Say HELLO"}, 1},
		{`startswith(output, "a")`, evaluator.Target{Output: "abc"}, 1},
		{`endswith(output, "z")`, evaluator.Target{Output: "abc"}, 0},
		{`count(output, "a")`, evaluator.Target{Output: "banana"}, 3},
		{`matches(output, "^[0-9]+$")`, evaluator.Target{Output: "12345"}, 1},
		{`round(similarity(output, expected), 2)`, evaluator.Target{Output: "a b c", Expected: "a b c"}, 1},
		{`json_valid(output)`, evaluator.Target{Output: `{"a":1}`}, 1},
		{`if(cost > 1, 0, 1)`, evaluator.Target{Cost: 0.5}, 1},
		{`min(1, 2) + max(3, 4)`, evaluator.Target{}, 5},
		{`abs(0 - 3)`, evaluator.Target{}, 3},
		{`number("2.5") * 2`, evaluator.Target{}, 5},
		{`!(cost > 1)`, evaluator.Target{Cost: 0.1}, 1},
		{`(1 + 2) * 3`, evaluator.Target{}, 9},
		{`7 % 3`, evaluator.Target{}, 1},
	}

	for _, tc := range cases {
		t.Run(tc.expression, func(t *testing.T) {
			impl := build(t, "custom_code", mustJSON(map[string]string{"expression": tc.expression}))
			if got := run(t, impl, tc.target); got.Score != tc.want {
				t.Errorf("expected %v, got %v (%s)", tc.want, got.Score, got.Reason)
			}
		})
	}
}

func TestCustomCodeClampsWhenAsked(t *testing.T) {
	impl := build(t, "custom_code", `{"expression":"5","clamp":true}`)
	if got := run(t, impl, evaluator.Target{}); got.Score != 1 {
		t.Errorf("expected the score to be clamped to 1, got %v", got.Score)
	}
}

func TestCustomCodeProducesCategoricalScores(t *testing.T) {
	impl := build(t, "custom_code", `{"expression":"if(cost > 0.01, \"expensive\", \"cheap\")"}`)

	result := run(t, impl, evaluator.Target{Cost: 0.5})
	if result.DataType != evaluator.DataTypeCategorical || result.StringValue != "expensive" {
		t.Errorf("expected a CATEGORICAL 'expensive', got %s/%q", result.DataType, result.StringValue)
	}
}

func TestCustomCodeReadsMetadata(t *testing.T) {
	impl := build(t, "custom_code", `{"expression":"metadata.retries"}`)
	result := run(t, impl, evaluator.Target{Metadata: map[string]interface{}{"retries": float64(3)}})
	if result.Score != 3 {
		t.Errorf("expected the metadata value, got %v", result.Score)
	}
}

func TestCustomCodeRejectsBadExpressions(t *testing.T) {
	cases := []string{
		`{"expression":"1 +"}`,
		`{"expression":"unknown_function(1)"}`,
		`{"expression":"(1 + 2"}`,
		`{"expression":""}`,
		`{"expression":"'unterminated"}`,
	}

	for _, config := range cases {
		if _, err := evaluator.Build("custom_code", json.RawMessage(config), evaluator.Deps{}); err == nil {
			t.Errorf("expected %s to be rejected", config)
		}
	}
}

func TestCustomCodeRejectsUnknownVariables(t *testing.T) {
	impl := build(t, "custom_code", `{"expression":"secret_value"}`)
	if _, err := impl.Evaluate(context.Background(), evaluator.Target{}); err == nil {
		t.Fatal("expected an unknown variable to fail at evaluation time")
	}
}

func TestCustomCodeRejectsDivisionByZero(t *testing.T) {
	impl := build(t, "custom_code", `{"expression":"1 / 0"}`)
	if _, err := impl.Evaluate(context.Background(), evaluator.Target{}); err == nil {
		t.Fatal("expected division by zero to error rather than produce infinity")
	}
}

func TestSemanticSimilarityMethods(t *testing.T) {
	identical := evaluator.Target{Output: "the quick brown fox", Expected: "the quick brown fox"}
	unrelated := evaluator.Target{Output: "the quick brown fox", Expected: "completely different words"}

	for _, method := range []string{"cosine", "jaccard", "levenshtein"} {
		t.Run(method, func(t *testing.T) {
			impl := build(t, "semantic_similarity", mustJSON(map[string]string{"method": method}))

			same := run(t, impl, identical)
			if same.Score < 0.999 {
				t.Errorf("expected identical text to score ~1, got %v", same.Score)
			}

			different := run(t, impl, unrelated)
			if different.Score >= same.Score {
				t.Errorf("expected unrelated text to score below identical text, got %v vs %v",
					different.Score, same.Score)
			}
		})
	}
}

func TestSemanticSimilarityThreshold(t *testing.T) {
	impl := build(t, "semantic_similarity", `{"threshold":0.9}`)

	if got := run(t, impl, evaluator.Target{Output: "same text", Expected: "same text"}); got.Score != 1 {
		t.Errorf("expected the threshold to be met, got %v", got.Score)
	}
	if got := run(t, impl, evaluator.Target{Output: "one", Expected: "two"}); got.Score != 0 {
		t.Errorf("expected the threshold to be missed, got %v", got.Score)
	}
}

func TestSemanticSimilarityRejectsAnUnknownMethod(t *testing.T) {
	if _, err := evaluator.Build("semantic_similarity", json.RawMessage(`{"method":"magic"}`), evaluator.Deps{}); err == nil {
		t.Fatal("expected an unknown method to be rejected")
	}
}

func TestLLMJudgeScoresFromModelReply(t *testing.T) {
	var received chatRequestBody

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected the API key to be sent, got %q", got)
		}
		_ = json.NewDecoder(r.Body).Decode(&received)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant",
			"content":"{\"score\": 8, \"reasoning\": \"clear and correct\"}"}}]}`))
	}))
	defer server.Close()

	impl := build(t, "llm_judge", mustJSON(map[string]interface{}{
		"api_url":   server.URL,
		"api_key":   "test-key",
		"model":     "gpt-4o-mini",
		"criteria":  "Is the answer correct and clear?",
		"max_score": 10,
	}))

	result := run(t, impl, evaluator.Target{Input: "2+2?", Output: "4", Expected: "4"})

	if result.Score != 8 {
		t.Errorf("expected the judge's score of 8, got %v", result.Score)
	}
	if result.Reason != "clear and correct" {
		t.Errorf("expected the judge's explanation, got %q", result.Reason)
	}
	if received.Model != "gpt-4o-mini" {
		t.Errorf("expected the configured model to be requested, got %q", received.Model)
	}
	if len(received.Messages) != 1 {
		t.Fatalf("expected one message, got %d", len(received.Messages))
	}

	// The prompt must carry the material being judged, or the judge is scoring nothing.
	prompt := received.Messages[0].Content
	for _, fragment := range []string{"2+2?", "4", "Is the answer correct and clear?"} {
		if !strings.Contains(prompt, fragment) {
			t.Errorf("expected the prompt to contain %q, got %q", fragment, prompt)
		}
	}
}

func TestLLMJudgeNormalizesScores(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"score\": 5}"}}]}`))
	}))
	defer server.Close()

	impl := build(t, "llm_judge", mustJSON(map[string]interface{}{
		"api_url": server.URL, "model": "m", "criteria": "c",
		"max_score": 10, "normalize": true,
	}))

	if got := run(t, impl, evaluator.Target{}); got.Score != 0.5 {
		t.Errorf("expected 5/10 normalized to 0.5, got %v", got.Score)
	}
}

func TestLLMJudgeClampsOutOfRangeScores(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"score\": 99}"}}]}`))
	}))
	defer server.Close()

	impl := build(t, "llm_judge", mustJSON(map[string]interface{}{
		"api_url": server.URL, "model": "m", "criteria": "c", "max_score": 10,
	}))

	result := run(t, impl, evaluator.Target{})
	if result.Score != 10 {
		t.Errorf("expected an out-of-range score to be clamped to 10, got %v", result.Score)
	}
	if !strings.Contains(result.Reason, "clamped") {
		t.Errorf("expected the clamp to be recorded in the reason, got %q", result.Reason)
	}
}

func TestLLMJudgeFallsBackToABareNumber(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"I would rate this a 7 out of 10."}}]}`))
	}))
	defer server.Close()

	impl := build(t, "llm_judge", mustJSON(map[string]interface{}{
		"api_url": server.URL, "model": "m", "criteria": "c", "max_score": 10,
	}))

	if got := run(t, impl, evaluator.Target{}); got.Score != 7 {
		t.Errorf("expected the bare number to be recovered, got %v", got.Score)
	}
}

func TestLLMJudgeCategorical(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":
			"{\"category\": \"harmful\", \"reasoning\": \"contains unsafe advice\"}"}}]}`))
	}))
	defer server.Close()

	impl := build(t, "llm_judge", mustJSON(map[string]interface{}{
		"api_url": server.URL, "model": "m", "criteria": "safety",
		"categories": []string{"safe", "harmful"},
	}))

	result := run(t, impl, evaluator.Target{})
	if result.DataType != evaluator.DataTypeCategorical {
		t.Errorf("expected a CATEGORICAL score, got %s", result.DataType)
	}
	if result.StringValue != "harmful" {
		t.Errorf("expected the label 'harmful', got %q", result.StringValue)
	}
}

func TestLLMJudgeSurfacesAPIErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer server.Close()

	impl := build(t, "llm_judge", mustJSON(map[string]interface{}{
		"api_url": server.URL, "model": "m", "criteria": "c",
	}))

	if _, err := impl.Evaluate(context.Background(), evaluator.Target{}); err == nil {
		t.Fatal("expected an upstream error to be reported rather than scored")
	}
}

func TestLLMJudgeRequiresConfiguration(t *testing.T) {
	cases := []string{
		`{"model":"m","criteria":"c"}`,
		`{"api_url":"http://x","criteria":"c"}`,
		`{"api_url":"http://x","model":"m"}`,
	}
	for _, config := range cases {
		if _, err := evaluator.Build("llm_judge", json.RawMessage(config), evaluator.Deps{}); err == nil {
			t.Errorf("expected %s to be rejected", config)
		}
	}
}

func TestEmbeddingSimilarity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.Input) != 2 {
			t.Errorf("expected two inputs, got %d", len(req.Input))
		}
		// Two identical unit vectors: cosine similarity is exactly 1.
		_, _ = w.Write([]byte(`{"data":[
			{"index":0,"embedding":[1,0,0]},
			{"index":1,"embedding":[1,0,0]}
		]}`))
	}))
	defer server.Close()

	impl := build(t, "embedding_similarity", mustJSON(map[string]interface{}{
		"api_url": server.URL, "model": "text-embedding-3-small",
	}))

	result := run(t, impl, evaluator.Target{Output: "a", Expected: "b"})
	if result.Score < 0.999 {
		t.Errorf("expected identical vectors to score 1, got %v", result.Score)
	}
}

func TestEmbeddingSimilarityOrthogonalVectors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
			{"index":0,"embedding":[1,0]},
			{"index":1,"embedding":[0,1]}
		]}`))
	}))
	defer server.Close()

	impl := build(t, "embedding_similarity", mustJSON(map[string]interface{}{
		"api_url": server.URL, "model": "m",
	}))

	if got := run(t, impl, evaluator.Target{Output: "a", Expected: "b"}); got.Score != 0 {
		t.Errorf("expected orthogonal vectors to score 0, got %v", got.Score)
	}
}

// chatRequestBody mirrors the request the judge sends, for assertions.
type chatRequestBody struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func mustJSON(v interface{}) string {
	encoded, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}
