package evaluator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

func init() {
	Register("llm_judge",
		"Scores output with an LLM against configured criteria, returning a score and explanation.",
		newLLMJudge)
	Register("embedding_similarity",
		"Scores cosine similarity between embeddings of two fields, via an embeddings API.",
		newEmbeddingSimilarity)

	// The `type` column stored LLM_JUDGE before evaluators were named.
	RegisterAlias("llm_judge_legacy", "llm_judge")
}

// defaultJudgePrompt is used when a configuration supplies criteria but no
// prompt of its own. It asks for strict JSON so the reply parses reliably.
const defaultJudgePrompt = `You are an impartial evaluator. Score the assistant's output against the criteria.

Criteria:
{{criteria}}

Input given to the assistant:
{{input}}

Assistant output:
{{output}}

Expected output (may be empty):
{{expected}}

Respond with JSON only, no prose and no code fence:
{"score": <number between {{min_score}} and {{max_score}}>, "reasoning": "<one or two sentences>"}`

// llmJudgeConfig configures the LLM-as-a-judge evaluator.
type llmJudgeConfig struct {
	// APIURL is an OpenAI-compatible chat completions endpoint. This works with
	// OpenAI, Azure OpenAI, Ollama, vLLM, LiteLLM and similar gateways.
	APIURL string `json:"api_url"`

	// APIKey is sent as a bearer token. APIKeyEnv names an environment variable
	// to read it from instead, so a key need not be stored in the database.
	APIKey    string `json:"api_key"`
	APIKeyEnv string `json:"api_key_env"`

	Model       string  `json:"model"`
	Prompt      string  `json:"prompt"`
	Criteria    string  `json:"criteria"`
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`

	MinScore *float64 `json:"min_score"`
	MaxScore *float64 `json:"max_score"`

	// Normalize rescales the model's score into 0..1, so judges with different
	// scales stay comparable on one dashboard.
	Normalize bool `json:"normalize"`

	// Categories, when set, makes this a classifier: the model picks one label
	// and the score is stored as CATEGORICAL.
	Categories []string `json:"categories"`
}

type llmJudgeEvaluator struct {
	config     llmJudgeConfig
	httpClient *http.Client
	minScore   float64
	maxScore   float64
}

func newLLMJudge(config json.RawMessage, deps Deps) (Evaluator, error) {
	var c llmJudgeConfig
	if err := json.Unmarshal(config, &c); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if c.APIURL == "" {
		return nil, fmt.Errorf("`api_url` is required (an OpenAI-compatible chat completions endpoint)")
	}
	if c.Model == "" {
		return nil, fmt.Errorf("`model` is required")
	}
	if c.Prompt == "" && c.Criteria == "" {
		return nil, fmt.Errorf("one of `prompt` or `criteria` is required")
	}
	if c.Prompt == "" {
		c.Prompt = defaultJudgePrompt
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = 512
	}

	minScore := 0.0
	if c.MinScore != nil {
		minScore = *c.MinScore
	}
	maxScore := 10.0
	if c.MaxScore != nil {
		maxScore = *c.MaxScore
	}
	if maxScore <= minScore {
		return nil, fmt.Errorf("`max_score` must be greater than `min_score`")
	}

	client := deps.HTTPClient
	if client == nil {
		client = DefaultHTTPClient(0)
	}

	return &llmJudgeEvaluator{
		config:     c,
		httpClient: client,
		minScore:   minScore,
		maxScore:   maxScore,
	}, nil
}

func (e *llmJudgeEvaluator) Evaluate(ctx context.Context, target Target) (Result, error) {
	prompt := e.renderPrompt(target)

	content, err := e.complete(ctx, prompt)
	if err != nil {
		return Result{}, err
	}

	if len(e.config.Categories) > 0 {
		return e.parseCategorical(content)
	}
	return e.parseNumeric(content)
}

// renderPrompt substitutes the target into the configured template.
func (e *llmJudgeEvaluator) renderPrompt(target Target) string {
	replacements := []string{
		"{{input}}", target.Input,
		"{{output}}", target.Output,
		"{{expected}}", target.Expected,
		"{{expected_output}}", target.Expected,
		"{{criteria}}", e.config.Criteria,
		"{{model}}", target.Model,
		"{{min_score}}", strconv.FormatFloat(e.minScore, 'g', -1, 64),
		"{{max_score}}", strconv.FormatFloat(e.maxScore, 'g', -1, 64),
		"{{categories}}", strings.Join(e.config.Categories, ", "),
	}
	return strings.NewReplacer(replacements...).Replace(e.config.Prompt)
}

// chatRequest is the OpenAI-compatible chat completions request body.
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse covers the subset of the response we read.
type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// complete calls the configured chat endpoint and returns the reply text.
func (e *llmJudgeEvaluator) complete(ctx context.Context, prompt string) (string, error) {
	payload, err := json.Marshal(chatRequest{
		Model:       e.config.Model,
		Messages:    []chatMessage{{Role: "user", Content: prompt}},
		Temperature: e.config.Temperature,
		MaxTokens:   e.config.MaxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("encoding judge request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.config.APIURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("building judge request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if key := resolveAPIKey(e.config.APIKey, e.config.APIKeyEnv); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling judge model: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading judge response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("judge model returned %d: %s", resp.StatusCode, truncate(string(body), 300))
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("decoding judge response: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("judge model error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("judge model returned no choices")
	}

	return parsed.Choices[0].Message.Content, nil
}

// judgeVerdict is the JSON the judge is asked to produce.
type judgeVerdict struct {
	Score     *float64 `json:"score"`
	Reasoning string   `json:"reasoning"`
	Reason    string   `json:"reason"`
	Category  string   `json:"category"`
	Label     string   `json:"label"`
}

// bareNumber matches a score when the model ignored the JSON instruction.
var bareNumber = regexp.MustCompile(`-?\d+(\.\d+)?`)

// parseNumeric reads a numeric verdict, falling back to the first number in the
// reply when the model did not honour the JSON contract.
func (e *llmJudgeEvaluator) parseNumeric(content string) (Result, error) {
	verdict, parsed := decodeVerdict(content)

	var raw float64
	switch {
	case parsed && verdict.Score != nil:
		raw = *verdict.Score
	default:
		match := bareNumber.FindString(content)
		if match == "" {
			return Result{}, fmt.Errorf("judge reply contained no score: %s", truncate(content, 200))
		}
		value, err := strconv.ParseFloat(match, 64)
		if err != nil {
			return Result{}, fmt.Errorf("judge reply contained no parseable score: %s", truncate(content, 200))
		}
		raw = value
	}

	// A model that ignores the range must not poison the aggregates.
	clamped := math.Max(e.minScore, math.Min(e.maxScore, raw))

	score := clamped
	if e.config.Normalize {
		score = (clamped - e.minScore) / (e.maxScore - e.minScore)
	}

	reason := firstNonEmpty(verdict.Reasoning, verdict.Reason)
	if reason == "" {
		reason = truncate(strings.TrimSpace(content), 300)
	}
	if clamped != raw {
		reason = fmt.Sprintf("%s (raw score %g clamped to [%g, %g])", reason, raw, e.minScore, e.maxScore)
	}

	return numeric(score, "%s", reason), nil
}

// parseCategorical reads a label verdict and checks it against the allow-list.
func (e *llmJudgeEvaluator) parseCategorical(content string) (Result, error) {
	verdict, _ := decodeVerdict(content)

	label := firstNonEmpty(verdict.Category, verdict.Label)
	if label == "" {
		// Fall back to whichever configured category the reply mentions.
		lowered := strings.ToLower(content)
		for _, category := range e.config.Categories {
			if strings.Contains(lowered, strings.ToLower(category)) {
				label = category
				break
			}
		}
	}
	if label == "" {
		return Result{}, fmt.Errorf("judge reply contained no category: %s", truncate(content, 200))
	}

	matched := ""
	for _, category := range e.config.Categories {
		if strings.EqualFold(category, label) {
			matched = category
			break
		}
	}
	if matched == "" {
		return Result{}, fmt.Errorf("judge returned category %q, which is not in the configured list", label)
	}

	reason := firstNonEmpty(verdict.Reasoning, verdict.Reason)
	if reason == "" {
		reason = "categorised as " + matched
	}

	return Result{
		StringValue: matched,
		DataType:    DataTypeCategorical,
		Reason:      reason,
	}, nil
}

// decodeVerdict parses the judge reply as JSON, tolerating a code fence or
// surrounding prose.
func decodeVerdict(content string) (judgeVerdict, bool) {
	candidate := stripCodeFence(content)

	var verdict judgeVerdict
	if err := json.Unmarshal([]byte(candidate), &verdict); err == nil {
		return verdict, true
	}

	// Retry on the outermost JSON object embedded in the reply.
	start := strings.IndexByte(candidate, '{')
	end := strings.LastIndexByte(candidate, '}')
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(candidate[start:end+1]), &verdict); err == nil {
			return verdict, true
		}
	}

	return judgeVerdict{}, false
}

// --- embedding similarity -------------------------------------------------

type embeddingSimilarityConfig struct {
	APIURL    string `json:"api_url"`
	APIKey    string `json:"api_key"`
	APIKeyEnv string `json:"api_key_env"`
	Model     string `json:"model"`

	Left  string `json:"left"`
	Right string `json:"right"`

	Threshold *float64 `json:"threshold"`
}

type embeddingSimilarityEvaluator struct {
	config     embeddingSimilarityConfig
	httpClient *http.Client
}

func newEmbeddingSimilarity(config json.RawMessage, deps Deps) (Evaluator, error) {
	var c embeddingSimilarityConfig
	if err := json.Unmarshal(config, &c); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if c.APIURL == "" {
		return nil, fmt.Errorf("`api_url` is required (an OpenAI-compatible embeddings endpoint)")
	}
	if c.Model == "" {
		return nil, fmt.Errorf("`model` is required")
	}
	if c.Left == "" {
		c.Left = "output"
	}
	if c.Right == "" {
		c.Right = "expected"
	}

	client := deps.HTTPClient
	if client == nil {
		client = DefaultHTTPClient(0)
	}

	return &embeddingSimilarityEvaluator{config: c, httpClient: client}, nil
}

func (e *embeddingSimilarityEvaluator) Evaluate(ctx context.Context, target Target) (Result, error) {
	left := fieldOf(target, e.config.Left)
	right := fieldOf(target, e.config.Right)

	if left == "" || right == "" {
		return Result{}, fmt.Errorf("embedding_similarity needs both %q and %q to be non-empty",
			e.config.Left, e.config.Right)
	}

	vectors, err := e.embed(ctx, []string{left, right})
	if err != nil {
		return Result{}, err
	}
	if len(vectors) < 2 {
		return Result{}, fmt.Errorf("embeddings API returned %d vectors, expected 2", len(vectors))
	}

	similarity, err := cosine(vectors[0], vectors[1])
	if err != nil {
		return Result{}, err
	}

	if e.config.Threshold != nil {
		return boolean(similarity >= *e.config.Threshold,
			"embedding similarity %.4f (threshold %.4f)", similarity, *e.config.Threshold), nil
	}
	return numeric(similarity, "embedding similarity %.4f", similarity), nil
}

// embeddingRequest is the OpenAI-compatible embeddings request body.
type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// embed returns one vector per input, in input order.
func (e *embeddingSimilarityEvaluator) embed(ctx context.Context, inputs []string) ([][]float64, error) {
	payload, err := json.Marshal(embeddingRequest{Model: e.config.Model, Input: inputs})
	if err != nil {
		return nil, fmt.Errorf("encoding embeddings request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.config.APIURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("building embeddings request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if key := resolveAPIKey(e.config.APIKey, e.config.APIKeyEnv); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling embeddings API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("reading embeddings response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embeddings API returned %d: %s", resp.StatusCode, truncate(string(body), 300))
	}

	var parsed embeddingResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decoding embeddings response: %w", err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("embeddings API error: %s", parsed.Error.Message)
	}

	// The API may return results out of order; place each by its index.
	vectors := make([][]float64, len(inputs))
	for _, item := range parsed.Data {
		if item.Index >= 0 && item.Index < len(vectors) {
			vectors[item.Index] = item.Embedding
		}
	}
	for i, v := range vectors {
		if len(v) == 0 {
			return nil, fmt.Errorf("embeddings API returned no vector for input %d", i)
		}
	}

	return vectors, nil
}

// cosine returns the cosine similarity of two equal-length vectors.
func cosine(a, b []float64) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("embedding dimensions differ (%d vs %d)", len(a), len(b))
	}

	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0, nil
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB)), nil
}

// --- shared ---------------------------------------------------------------

// resolveAPIKey prefers an environment variable so credentials can be kept out
// of the evaluator configuration stored in the database.
func resolveAPIKey(literal, envName string) string {
	if envName != "" {
		if value := os.Getenv(envName); value != "" {
			return value
		}
	}
	return literal
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
