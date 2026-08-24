package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/langfuse-light/langfuse-light/internal/db"
)

// Evaluator evaluates a trace and returns a score.
type Evaluator interface {
	Evaluate(ctx context.Context, trace db.Trace, observations []db.Observation) (float64, string, error)
}

// EvaluatorConfigService handles evaluator configuration and runs.
type EvaluatorConfigService struct {
	queries *db.Queries
}

// NewEvaluatorConfigService creates a new evaluator config service.
func NewEvaluatorConfigService(queries *db.Queries) *EvaluatorConfigService {
	return &EvaluatorConfigService{queries: queries}
}

// EvaluatorType represents the type of evaluator.
type EvaluatorType string

const (
	EvaluatorTypeCode     EvaluatorType = "CODE"
	EvaluatorTypeLLMJudge EvaluatorType = "LLM_JUDGE"
)

// CreateEvaluatorConfigRequest is the request body for creating an evaluator config.
type CreateEvaluatorConfigRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Type        EvaluatorType   `json:"type"`
	Config      json.RawMessage `json:"config"`
}

// CreateEvaluatorConfig creates a new evaluator configuration.
func (s *EvaluatorConfigService) CreateEvaluatorConfig(ctx context.Context, projectID string, req CreateEvaluatorConfigRequest) (db.EvaluatorConfig, error) {
	if req.Name == "" {
		return db.EvaluatorConfig{}, fmt.Errorf("name is required")
	}
	if req.Type == "" {
		req.Type = EvaluatorTypeCode
	}
	if req.Config == nil {
		req.Config = json.RawMessage("{}")
	}

	config, err := s.queries.CreateEvaluatorConfig(ctx, db.CreateEvaluatorConfigParams{
		ProjectID:   projectID,
		Name:        req.Name,
		Description: pgtype.Text{String: req.Description, Valid: req.Description != ""},
		Type:        string(req.Type),
		Config:      req.Config,
		IsActive:    true,
	})
	if err != nil {
		return db.EvaluatorConfig{}, fmt.Errorf("creating evaluator config: %w", err)
	}

	return config, nil
}

// ListEvaluatorConfigs returns all evaluator configs for a project.
func (s *EvaluatorConfigService) ListEvaluatorConfigs(ctx context.Context, projectID string) ([]db.EvaluatorConfig, error) {
	configs, err := s.queries.GetEvaluatorConfigsByProjectID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("listing evaluator configs: %w", err)
	}
	return configs, nil
}

// GetEvaluatorConfig returns an evaluator config by ID.
func (s *EvaluatorConfigService) GetEvaluatorConfig(ctx context.Context, id string) (db.EvaluatorConfig, error) {
	config, err := s.queries.GetEvaluatorConfigByID(ctx, id)
	if err != nil {
		return db.EvaluatorConfig{}, fmt.Errorf("getting evaluator config: %w", err)
	}
	return config, nil
}

// DeleteEvaluatorConfig deletes an evaluator config by ID.
func (s *EvaluatorConfigService) DeleteEvaluatorConfig(ctx context.Context, id string) error {
	if err := s.queries.DeleteEvaluatorConfig(ctx, id); err != nil {
		return fmt.Errorf("deleting evaluator config: %w", err)
	}
	return nil
}

// EvaluateTracesRequest is the request body for running an evaluation.
type EvaluateTracesRequest struct {
	TraceIDs []string `json:"trace_ids"`
}

// EvaluationResult represents the result of evaluating a single trace.
type EvaluationResult struct {
	TraceID string  `json:"trace_id"`
	Score   float64 `json:"score"`
	Reason  string  `json:"reason"`
}

// EvaluateTraces runs an evaluator against the specified traces and creates scores.
func (s *EvaluatorConfigService) EvaluateTraces(ctx context.Context, evaluatorID string, req EvaluateTracesRequest) (db.EvaluationRun, error) {
	config, err := s.queries.GetEvaluatorConfigByID(ctx, evaluatorID)
	if err != nil {
		return db.EvaluationRun{}, fmt.Errorf("getting evaluator config: %w", err)
	}

	// Create the evaluation run
	run, err := s.queries.CreateEvaluationRun(ctx, db.CreateEvaluationRunParams{
		ProjectID:         config.ProjectID,
		EvaluatorConfigID: evaluatorID,
		Name:              fmt.Sprintf("run-%s", config.Name),
		Status:            "RUNNING",
	})
	if err != nil {
		return db.EvaluationRun{}, fmt.Errorf("creating evaluation run: %w", err)
	}

	evaluator, err := buildEvaluator(config.Type, config.Config)
	if err != nil {
		s.queries.UpdateEvaluationRunStatus(ctx, db.UpdateEvaluationRunStatusParams{
			ID:     run.ID,
			Status: "FAILED",
		})
		return run, fmt.Errorf("building evaluator: %w", err)
	}

	var results []EvaluationResult
	successCount := 0
	failCount := 0

	for _, traceID := range req.TraceIDs {
		trace, err := s.queries.GetTraceByID(ctx, traceID)
		if err != nil {
			failCount++
			continue
		}

		observations, err := s.queries.GetObservationsByTraceID(ctx, traceID)
		if err != nil {
			observations = nil
		}

		score, reason, err := evaluator.Evaluate(ctx, trace, observations)
		if err != nil {
			failCount++
			continue
		}

		// Create score in DB
		_, err = s.queries.CreateScore(ctx, db.CreateScoreParams{
			TraceID: traceID,
			Name:    config.Name,
			Value:   pgtype.Float8{Float64: score, Valid: true},
			Comment: pgtype.Text{String: reason, Valid: reason != ""},
			Source:  "EVALUATOR",
		})
		if err != nil {
			failCount++
			continue
		}

		results = append(results, EvaluationResult{
			TraceID: traceID,
			Score:   score,
			Reason:  reason,
		})
		successCount++
	}

	summary := map[string]interface{}{
		"total":     len(req.TraceIDs),
		"success":   successCount,
		"failed":    failCount,
		"avg_score": avgScore(results),
	}
	summaryJSON, _ := json.Marshal(summary)

	finalStatus := "COMPLETED"
	if failCount == len(req.TraceIDs) {
		finalStatus = "FAILED"
	}

	s.queries.UpdateEvaluationRunStatus(ctx, db.UpdateEvaluationRunStatusParams{
		ID:            run.ID,
		Status:        finalStatus,
		ResultSummary: summaryJSON,
	})

	run.Status = finalStatus
	return run, nil
}

// ListEvaluationRuns returns all evaluation runs for a project.
func (s *EvaluatorConfigService) ListEvaluationRuns(ctx context.Context, projectID string) ([]db.EvaluationRun, error) {
	runs, err := s.queries.GetEvaluationRunsByProjectID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("listing evaluation runs: %w", err)
	}
	return runs, nil
}

// GetEvaluationRun returns an evaluation run by ID.
func (s *EvaluatorConfigService) GetEvaluationRun(ctx context.Context, id string) (db.EvaluationRun, error) {
	run, err := s.queries.GetEvaluationRunByID(ctx, id)
	if err != nil {
		return db.EvaluationRun{}, fmt.Errorf("getting evaluation run: %w", err)
	}
	return run, nil
}

func avgScore(results []EvaluationResult) float64 {
	if len(results) == 0 {
		return 0
	}
	sum := 0.0
	for _, r := range results {
		sum += r.Score
	}
	return math.Round(sum/float64(len(results))*1000) / 1000
}

// BuildEvaluatorForTest creates an evaluator from type and config (exported for testing).
func BuildEvaluatorForTest(evalType string, config json.RawMessage) (Evaluator, error) {
	return buildEvaluator(evalType, config)
}

func buildEvaluator(evalType string, config json.RawMessage) (Evaluator, error) {
	switch EvaluatorType(evalType) {
	case EvaluatorTypeCode:
		return buildCodeEvaluator(config)
	case EvaluatorTypeLLMJudge:
		return buildLLMJudgeEvaluator(config)
	default:
		return nil, fmt.Errorf("unknown evaluator type: %s", evalType)
	}
}

// Code evaluator types
type lengthCheckConfig struct {
	MinLength int    `json:"min_length"`
	MaxLength int    `json:"max_length"`
	Field     string `json:"field"` // "input" or "output"
}

type keywordCheckConfig struct {
	Keywords []string `json:"keywords"`
	Field    string   `json:"field"`
	Mode     string   `json:"mode"` // "contains" or "not_contains"
}

type regexCheckConfig struct {
	Pattern string `json:"pattern"`
	Field   string `json:"field"`
}

type numericRangeConfig struct {
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Field string  `json:"field"` // "cost" or "tokens"
}

func buildCodeEvaluator(config json.RawMessage) (Evaluator, error) {
	var cfg map[string]interface{}
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parsing code evaluator config: %w", err)
	}

	evalType, _ := cfg["evaluator"].(string)
	switch evalType {
	case "length_check":
		var c lengthCheckConfig
		json.Unmarshal(config, &c)
		return &lengthCheckEvaluator{config: c}, nil
	case "keyword_check":
		var c keywordCheckConfig
		json.Unmarshal(config, &c)
		return &keywordCheckEvaluator{config: c}, nil
	case "regex_check":
		var c regexCheckConfig
		json.Unmarshal(config, &c)
		return &regexCheckEvaluator{config: c}, nil
	case "numeric_range":
		var c numericRangeConfig
		json.Unmarshal(config, &c)
		return &numericRangeEvaluator{config: c}, nil
	default:
		return &noopEvaluator{}, nil
	}
}

// LLM Judge evaluator
type llmJudgeConfig struct {
	APIURL   string  `json:"api_url"`
	APIKey   string  `json:"api_key"`
	Model    string  `json:"model"`
	Prompt   string  `json:"prompt"`
	MaxScore float64 `json:"max_score"`
}

func buildLLMJudgeEvaluator(config json.RawMessage) (Evaluator, error) {
	var c llmJudgeConfig
	if err := json.Unmarshal(config, &c); err != nil {
		return nil, fmt.Errorf("parsing LLM judge config: %w", err)
	}
	if c.MaxScore == 0 {
		c.MaxScore = 10
	}
	return &llmJudgeEvaluator{config: c}, nil
}

// --- Built-in evaluators ---

type noopEvaluator struct{}

func (e *noopEvaluator) Evaluate(_ context.Context, _ db.Trace, _ []db.Observation) (float64, string, error) {
	return 0, "noop evaluator", nil
}

type lengthCheckEvaluator struct {
	config lengthCheckConfig
}

func (e *lengthCheckEvaluator) Evaluate(_ context.Context, trace db.Trace, _ []db.Observation) (float64, string, error) {
	var text string
	switch e.config.Field {
	case "output":
		text = string(trace.Output)
	default:
		text = string(trace.Input)
	}

	length := len(text)
	score := 1.0
	reason := fmt.Sprintf("length=%d", length)

	if e.config.MinLength > 0 && length < e.config.MinLength {
		score = 0
		reason = fmt.Sprintf("length=%d, below minimum %d", length, e.config.MinLength)
	}
	if e.config.MaxLength > 0 && length > e.config.MaxLength {
		score = 0
		reason = fmt.Sprintf("length=%d, above maximum %d", length, e.config.MaxLength)
	}

	return score, reason, nil
}

type keywordCheckEvaluator struct {
	config keywordCheckConfig
}

func (e *keywordCheckEvaluator) Evaluate(_ context.Context, trace db.Trace, _ []db.Observation) (float64, string, error) {
	var text string
	switch e.config.Field {
	case "output":
		text = string(trace.Output)
	default:
		text = string(trace.Input)
	}

	text = strings.ToLower(text)
	found := 0
	for _, kw := range e.config.Keywords {
		if strings.Contains(text, strings.ToLower(kw)) {
			found++
		}
	}

	score := float64(found) / float64(len(e.config.Keywords))
	if e.config.Mode == "not_contains" {
		score = 1 - score
	}

	reason := fmt.Sprintf("found %d/%d keywords", found, len(e.config.Keywords))
	return score, reason, nil
}

type regexCheckEvaluator struct {
	config regexCheckConfig
}

func (e *regexCheckEvaluator) Evaluate(_ context.Context, trace db.Trace, _ []db.Observation) (float64, string, error) {
	var text string
	switch e.config.Field {
	case "output":
		text = string(trace.Output)
	default:
		text = string(trace.Input)
	}

	matched, err := regexp.MatchString(e.config.Pattern, text)
	if err != nil {
		return 0, fmt.Sprintf("invalid regex: %v", err), nil
	}

	score := 0.0
	reason := fmt.Sprintf("pattern=%s, matched=%v", e.config.Pattern, matched)
	if matched {
		score = 1
	}

	return score, reason, nil
}

type numericRangeEvaluator struct {
	config numericRangeConfig
}

func (e *numericRangeEvaluator) Evaluate(_ context.Context, trace db.Trace, _ []db.Observation) (float64, string, error) {
	var value float64
	switch e.config.Field {
	case "cost":
		if trace.TotalCost.Valid {
			value = trace.TotalCost.Float64
		}
	default:
		return 0, "unsupported field for numeric range", nil
	}

	score := 0.0
	reason := fmt.Sprintf("value=%.6f, range=[%.6f, %.6f]", value, e.config.Min, e.config.Max)
	if value >= e.config.Min && value <= e.config.Max {
		score = 1
		reason = fmt.Sprintf("value=%.6f within range", value)
	}

	return score, reason, nil
}

// LLM Judge evaluator — calls an external LLM API
type llmJudgeEvaluator struct {
	config llmJudgeConfig
}

func (e *llmJudgeEvaluator) Evaluate(ctx context.Context, trace db.Trace, _ []db.Observation) (float64, string, error) {
	if e.config.APIURL == "" {
		return 0, "LLM judge requires api_url in config", fmt.Errorf("missing api_url")
	}

	// Build the prompt with trace data
	prompt := e.config.Prompt
	prompt = strings.ReplaceAll(prompt, "{{input}}", string(trace.Input))
	prompt = strings.ReplaceAll(prompt, "{{output}}", string(trace.Output))

	// For now, return a placeholder — full LLM API integration would go here
	// This creates the score entry and can be extended with actual API calls
	return 0.5, fmt.Sprintf("LLM judge placeholder (model=%s)", e.config.Model), nil
}
