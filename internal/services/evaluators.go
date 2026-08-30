package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/obsevo/obsevo/internal/db"
	"github.com/obsevo/obsevo/internal/services/evaluator"
)

// maxTracesPerRun bounds a single evaluation run so one request cannot occupy
// the process indefinitely, which matters most for network-backed evaluators.
const maxTracesPerRun = 500

// EvaluatorConfigService handles evaluator configuration and runs.
type EvaluatorConfigService struct {
	queries *db.Queries
	deps    evaluator.Deps
}

// NewEvaluatorConfigService creates a new evaluator config service. The timeout
// bounds each outbound call made by network-backed evaluators.
func NewEvaluatorConfigService(queries *db.Queries, timeout time.Duration) *EvaluatorConfigService {
	return &EvaluatorConfigService{
		queries: queries,
		deps:    evaluator.Deps{HTTPClient: evaluator.DefaultHTTPClient(timeout)},
	}
}

// EvaluatorType is the stored evaluator kind. It is the registered evaluator's
// name; CODE and LLM_JUDGE are retained from before evaluators were named.
type EvaluatorType string

const (
	EvaluatorTypeCode     EvaluatorType = "CODE"
	EvaluatorTypeLLMJudge EvaluatorType = "LLM_JUDGE"
)

// AvailableEvaluators returns every registered evaluator with its description.
func AvailableEvaluators() []evaluator.Descriptor {
	return evaluator.Descriptors()
}

// CreateEvaluatorConfigRequest is the request body for creating an evaluator config.
type CreateEvaluatorConfigRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Type        EvaluatorType   `json:"type"`
	Config      json.RawMessage `json:"config"`
}

// CreateEvaluatorConfig creates a new evaluator configuration. The evaluator is
// built once here so an invalid configuration is rejected on write rather than
// silently scoring zero at run time.
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

	if _, err := s.buildEvaluator(string(req.Type), req.Config); err != nil {
		return db.EvaluatorConfig{}, err
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

// GetEvaluatorConfig returns an evaluator config by ID, scoped to a project.
func (s *EvaluatorConfigService) GetEvaluatorConfig(ctx context.Context, projectID, id string) (db.EvaluatorConfig, error) {
	config, err := s.queries.GetEvaluatorConfigByIDAndProject(ctx, db.GetEvaluatorConfigByIDAndProjectParams{
		ID:        id,
		ProjectID: projectID,
	})
	if err != nil {
		return db.EvaluatorConfig{}, fmt.Errorf("getting evaluator config: %w", err)
	}
	return config, nil
}

// DeleteEvaluatorConfig deletes an evaluator config, scoped to a project.
func (s *EvaluatorConfigService) DeleteEvaluatorConfig(ctx context.Context, projectID, id string) error {
	err := s.queries.DeleteEvaluatorConfig(ctx, db.DeleteEvaluatorConfigParams{
		ID:        id,
		ProjectID: projectID,
	})
	if err != nil {
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
	TraceID     string  `json:"trace_id"`
	Score       float64 `json:"score"`
	StringValue string  `json:"string_value,omitempty"`
	DataType    string  `json:"data_type"`
	Reason      string  `json:"reason"`
	Error       string  `json:"error,omitempty"`
}

// EvaluateTraces runs an evaluator against the given traces and stores a score
// per trace. The run row records the outcome so partial failures are visible.
func (s *EvaluatorConfigService) EvaluateTraces(ctx context.Context, projectID, evaluatorID string, req EvaluateTracesRequest) (db.EvaluationRun, []EvaluationResult, error) {
	config, err := s.queries.GetEvaluatorConfigByIDAndProject(ctx, db.GetEvaluatorConfigByIDAndProjectParams{
		ID:        evaluatorID,
		ProjectID: projectID,
	})
	if err != nil {
		return db.EvaluationRun{}, nil, fmt.Errorf("getting evaluator config: %w", err)
	}

	if len(req.TraceIDs) == 0 {
		return db.EvaluationRun{}, nil, fmt.Errorf("trace_ids must not be empty")
	}
	if len(req.TraceIDs) > maxTracesPerRun {
		return db.EvaluationRun{}, nil, fmt.Errorf("at most %d traces can be evaluated per run", maxTracesPerRun)
	}

	run, err := s.queries.CreateEvaluationRun(ctx, db.CreateEvaluationRunParams{
		ProjectID:         config.ProjectID,
		EvaluatorConfigID: evaluatorID,
		Name:              fmt.Sprintf("run-%s", config.Name),
		Status:            "RUNNING",
	})
	if err != nil {
		return db.EvaluationRun{}, nil, fmt.Errorf("creating evaluation run: %w", err)
	}

	impl, err := s.buildEvaluator(config.Type, config.Config)
	if err != nil {
		s.finishRun(ctx, run.ID, "FAILED", map[string]interface{}{"error": err.Error()})
		run.Status = "FAILED"
		return run, nil, fmt.Errorf("building evaluator: %w", err)
	}

	results := make([]EvaluationResult, 0, len(req.TraceIDs))
	successCount := 0
	failCount := 0

	for _, traceID := range req.TraceIDs {
		result, err := s.evaluateOne(ctx, projectID, config, impl, traceID)
		if err != nil {
			failCount++
			results = append(results, EvaluationResult{TraceID: traceID, Error: err.Error()})
			continue
		}
		successCount++
		results = append(results, result)
	}

	status := "COMPLETED"
	if successCount == 0 {
		status = "FAILED"
	}
	s.finishRun(ctx, run.ID, status, map[string]interface{}{
		"total":     len(req.TraceIDs),
		"success":   successCount,
		"failed":    failCount,
		"avg_score": avgScore(results),
	})

	run.Status = status
	return run, results, nil
}

// evaluateOne scores a single trace and records the score.
func (s *EvaluatorConfigService) evaluateOne(
	ctx context.Context,
	projectID string,
	config db.EvaluatorConfig,
	impl evaluator.Evaluator,
	traceID string,
) (EvaluationResult, error) {
	trace, err := s.queries.GetTraceByIDAndProject(ctx, db.GetTraceByIDAndProjectParams{
		ID:        traceID,
		ProjectID: projectID,
	})
	if err != nil {
		return EvaluationResult{}, fmt.Errorf("loading trace: %w", err)
	}

	observations, err := s.queries.GetObservationsByTraceIDAndProject(ctx, db.GetObservationsByTraceIDAndProjectParams{
		TraceID:   traceID,
		ProjectID: projectID,
	})
	if err != nil {
		observations = nil
	}

	outcome, err := impl.Evaluate(ctx, BuildTarget(trace, observations, ""))
	if err != nil {
		return EvaluationResult{}, err
	}

	value := pgtype.Float8{Float64: outcome.Score, Valid: true}
	if outcome.DataType == evaluator.DataTypeCategorical && outcome.StringValue != "" {
		value = pgtype.Float8{}
	}

	_, err = s.queries.CreateScore(ctx, db.CreateScoreParams{
		ProjectID:   projectID,
		TraceID:     traceID,
		Name:        config.Name,
		Value:       value,
		StringValue: outcome.StringValue,
		DataType:    outcome.DataType,
		Comment:     outcome.Reason,
		Source:      "EVALUATOR",
	})
	if err != nil {
		return EvaluationResult{}, fmt.Errorf("recording score: %w", err)
	}

	return EvaluationResult{
		TraceID:     traceID,
		Score:       outcome.Score,
		StringValue: outcome.StringValue,
		DataType:    outcome.DataType,
		Reason:      outcome.Reason,
	}, nil
}

// finishRun records a run's terminal status and summary.
func (s *EvaluatorConfigService) finishRun(ctx context.Context, runID, status string, summary map[string]interface{}) {
	encoded, err := json.Marshal(summary)
	if err != nil {
		encoded = nil
	}
	if err := s.queries.UpdateEvaluationRunStatus(ctx, db.UpdateEvaluationRunStatusParams{
		ID:            runID,
		Status:        status,
		ResultSummary: encoded,
	}); err != nil {
		// The scores are already written; a failed status update must not lose them.
		return
	}
}

// ListEvaluationRuns returns all evaluation runs for a project.
func (s *EvaluatorConfigService) ListEvaluationRuns(ctx context.Context, projectID string) ([]db.EvaluationRun, error) {
	runs, err := s.queries.GetEvaluationRunsByProjectID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("listing evaluation runs: %w", err)
	}
	return runs, nil
}

// GetEvaluationRun returns an evaluation run by ID, scoped to a project.
func (s *EvaluatorConfigService) GetEvaluationRun(ctx context.Context, projectID, id string) (db.EvaluationRun, error) {
	run, err := s.queries.GetEvaluationRunByIDAndProject(ctx, db.GetEvaluationRunByIDAndProjectParams{
		ID:        id,
		ProjectID: projectID,
	})
	if err != nil {
		return db.EvaluationRun{}, fmt.Errorf("getting evaluation run: %w", err)
	}
	return run, nil
}

// BuildEvaluator constructs an evaluator from a stored type and configuration.
// It is exported so the experiment runner can score dataset items with the same
// evaluators used for traces.
func (s *EvaluatorConfigService) BuildEvaluator(evalType string, config json.RawMessage) (evaluator.Evaluator, error) {
	return s.buildEvaluator(evalType, config)
}

// buildEvaluator resolves a stored evaluator type to a registered evaluator.
//
// The legacy `CODE` type does not name an evaluator; the name lives in the
// config's `evaluator` field instead. That indirection is preserved so
// configurations written before the registry keep working, including their
// original behaviour of falling back to a no-op for an unrecognised name.
func (s *EvaluatorConfigService) buildEvaluator(evalType string, config json.RawMessage) (evaluator.Evaluator, error) {
	if strings.EqualFold(evalType, string(EvaluatorTypeCode)) {
		name := legacyEvaluatorName(config)
		if name == "" || !evaluator.Exists(name) {
			return noopEvaluator{}, nil
		}
		return evaluator.Build(name, config, s.deps)
	}

	return evaluator.Build(evalType, config, s.deps)
}

// legacyEvaluatorName reads the evaluator name out of a legacy CODE config.
func legacyEvaluatorName(config json.RawMessage) string {
	var fields struct {
		Evaluator string `json:"evaluator"`
	}
	if err := json.Unmarshal(config, &fields); err != nil {
		return ""
	}
	return fields.Evaluator
}

// noopEvaluator preserves the pre-registry behaviour for an unrecognised legacy
// CODE evaluator: score zero and say so, rather than fail the run.
type noopEvaluator struct{}

func (noopEvaluator) Evaluate(context.Context, evaluator.Target) (evaluator.Result, error) {
	return evaluator.Result{
		Score:    0,
		DataType: evaluator.DataTypeNumeric,
		Reason:   "noop evaluator",
	}, nil
}

// BuildTarget assembles the material an evaluator scores from a stored trace.
// expected carries a dataset item's expected output during an experiment, and
// is empty when scoring a production trace.
func BuildTarget(trace db.Trace, observations []db.Observation, expected string) evaluator.Target {
	target := evaluator.Target{
		Input:    decodeJSONText(trace.Input),
		Output:   decodeJSONText(trace.Output),
		Expected: expected,
		Metadata: decodeJSONObject(trace.Metadata),
	}

	if trace.TotalCost.Valid {
		target.Cost = trace.TotalCost.Float64
	}
	usage := parseTokenUsage(trace.TokenUsage)
	target.InputTokens = usage.InputTokens
	target.OutputTokens = usage.OutputTokens
	target.TotalTokens = usage.TotalTokens

	if trace.StartTime.Valid && trace.EndTime.Valid {
		target.LatencySeconds = trace.EndTime.Time.Sub(trace.StartTime.Time).Seconds()
	}

	target.ObservationCount = len(observations)
	for _, obs := range observations {
		if obs.Status == "ERROR" || obs.Level == "ERROR" {
			target.ErrorCount++
		}
		// A generation's model identifies the trace when the trace itself has none.
		if target.Model == "" && obs.Model.Valid {
			target.Model = obs.Model.String
		}
	}

	return target
}

// decodeJSONText renders a stored JSONB value as text: a JSON string is
// unquoted, anything else is returned as compact JSON.
func decodeJSONText(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}
	return string(raw)
}

// decodeJSONObject decodes a stored JSONB object, returning nil for anything else.
func decodeJSONObject(raw []byte) map[string]interface{} {
	if len(raw) == 0 {
		return nil
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	return obj
}

// avgScore averages the numeric results of a run, ignoring failures.
func avgScore(results []EvaluationResult) float64 {
	sum := 0.0
	count := 0
	for _, r := range results {
		if r.Error != "" || r.DataType == evaluator.DataTypeCategorical {
			continue
		}
		sum += r.Score
		count++
	}
	if count == 0 {
		return 0
	}
	return math.Round(sum/float64(count)*1000) / 1000
}

// --- backward-compatible shim --------------------------------------------

// Evaluator is the pre-registry evaluator interface, kept so existing callers
// and tests continue to compile. New code should use evaluator.Evaluator, which
// carries the richer target and result types.
type Evaluator interface {
	Evaluate(ctx context.Context, trace db.Trace, observations []db.Observation) (float64, string, error)
}

// legacyAdapter presents a registry evaluator through the old interface.
type legacyAdapter struct {
	inner evaluator.Evaluator
}

func (a legacyAdapter) Evaluate(ctx context.Context, trace db.Trace, observations []db.Observation) (float64, string, error) {
	result, err := a.inner.Evaluate(ctx, BuildTarget(trace, observations, ""))
	if err != nil {
		return 0, "", err
	}
	return result.Score, result.Reason, nil
}

// BuildEvaluatorForTest creates an evaluator from a stored type and config,
// exposed through the legacy interface for tests.
func BuildEvaluatorForTest(evalType string, config json.RawMessage) (Evaluator, error) {
	service := &EvaluatorConfigService{
		deps: evaluator.Deps{HTTPClient: &http.Client{Timeout: 30 * time.Second}},
	}
	impl, err := service.buildEvaluator(evalType, config)
	if err != nil {
		return nil, err
	}
	return legacyAdapter{inner: impl}, nil
}

// SortedEvaluatorNames returns the registered evaluator names in order.
func SortedEvaluatorNames() []string {
	names := evaluator.Names()
	sort.Strings(names)
	return names
}
