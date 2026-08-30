package services

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/obsevo/obsevo/internal/db"
)

// priceCacheTTL bounds how long a project's price list is reused before it is
// reloaded. Pricing changes rarely, and ingestion must not pay for a lookup on
// every observation.
const priceCacheTTL = 5 * time.Minute

// TokenUsage is the normalized token shape stored on traces and observations.
type TokenUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
}

// CostDetails breaks a computed cost down by token class.
type CostDetails struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
	Total  float64 `json:"total"`
}

// CostService prices generations from configurable per-model rates.
type CostService struct {
	queries *db.Queries

	mu     sync.RWMutex
	cache  map[string][]compiledPrice
	loaded map[string]time.Time
}

// compiledPrice is a model_prices row with its pattern pre-compiled.
type compiledPrice struct {
	pattern     *regexp.Regexp
	modelName   string
	inputPrice  float64
	outputPrice float64
	totalPrice  float64
	hasInput    bool
	hasOutput   bool
	hasTotal    bool
}

// NewCostService creates a new cost service.
func NewCostService(queries *db.Queries) *CostService {
	return &CostService{
		queries: queries,
		cache:   make(map[string][]compiledPrice),
		loaded:  make(map[string]time.Time),
	}
}

// ApplyCost fills in an observation's cost and cost breakdown from the model
// pricing table. A cost supplied by the client always wins — SDKs that already
// know the true billed amount should not have it overwritten by an estimate.
func (s *CostService) ApplyCost(ctx context.Context, projectID string, req *CreateObservationRequest) {
	if req.Cost != nil || req.Model == "" {
		return
	}

	usage := parseTokenUsage(req.TokenUsage)
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.TotalTokens == 0 {
		return
	}

	price, ok := s.lookup(ctx, projectID, req.Model)
	if !ok {
		return
	}

	details := price.compute(usage)
	if details.Total == 0 {
		return
	}

	total := details.Total
	req.Cost = &total
	if encoded, err := json.Marshal(details); err == nil {
		req.CostDetails = encoded
	}
}

// PriceObservation computes and stores an observation's cost from the row as
// it now stands in the database.
//
// Pricing has to happen after the write, not before it: an SDK reports a
// generation's model on the create event and its token usage on the update
// event, so neither event alone carries enough to price the call. The merged
// row does.
//
// It reports whether a cost was written. A client-supplied cost is never
// overwritten, and an observation with no model or no usage is left alone.
func (s *CostService) PriceObservation(ctx context.Context, projectID string, obs db.Observation) (bool, error) {
	if obs.Cost.Valid || !obs.Model.Valid || obs.Model.String == "" {
		return false, nil
	}

	usage := parseTokenUsage(obs.TokenUsage)
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.TotalTokens == 0 {
		return false, nil
	}

	price, ok := s.lookup(ctx, projectID, obs.Model.String)
	if !ok {
		return false, nil
	}

	details := price.compute(usage)
	if details.Total == 0 {
		return false, nil
	}

	encoded, err := json.Marshal(details)
	if err != nil {
		return false, fmt.Errorf("encoding cost details: %w", err)
	}

	err = s.queries.UpdateObservationCost(ctx, db.UpdateObservationCostParams{
		ID:          obs.ID,
		ProjectID:   projectID,
		Cost:        float8(details.Total),
		CostDetails: encoded,
	})
	if err != nil {
		return false, fmt.Errorf("storing observation cost: %w", err)
	}

	return true, nil
}

// EstimateCost returns the cost of a usage record under a project's pricing,
// without mutating anything. Used by experiment and dataset-run reporting.
func (s *CostService) EstimateCost(ctx context.Context, projectID, model string, usage TokenUsage) (CostDetails, bool) {
	if model == "" {
		return CostDetails{}, false
	}
	price, ok := s.lookup(ctx, projectID, model)
	if !ok {
		return CostDetails{}, false
	}
	return price.compute(usage), true
}

// compute applies a price row to a usage record.
func (p compiledPrice) compute(usage TokenUsage) CostDetails {
	var details CostDetails

	if p.hasInput {
		details.Input = float64(usage.InputTokens) * p.inputPrice
	}
	if p.hasOutput {
		details.Output = float64(usage.OutputTokens) * p.outputPrice
	}
	details.Total = details.Input + details.Output

	// A flat per-token price applies when no split rate is configured.
	if !p.hasInput && !p.hasOutput && p.hasTotal {
		total := usage.TotalTokens
		if total == 0 {
			total = usage.InputTokens + usage.OutputTokens
		}
		details.Total = float64(total) * p.totalPrice
	}

	return details
}

// lookup returns the most specific price matching a model name.
func (s *CostService) lookup(ctx context.Context, projectID, model string) (compiledPrice, bool) {
	prices, err := s.pricesFor(ctx, projectID)
	if err != nil {
		return compiledPrice{}, false
	}

	lowered := strings.ToLower(model)
	for _, p := range prices {
		if p.modelName != "" && strings.ToLower(p.modelName) == lowered {
			return p, true
		}
	}
	for _, p := range prices {
		if p.pattern != nil && p.pattern.MatchString(model) {
			return p, true
		}
	}
	return compiledPrice{}, false
}

// pricesFor returns a project's price list, reloading it when the cache expires.
func (s *CostService) pricesFor(ctx context.Context, projectID string) ([]compiledPrice, error) {
	s.mu.RLock()
	cached, ok := s.cache[projectID]
	loadedAt := s.loaded[projectID]
	s.mu.RUnlock()

	if ok && time.Since(loadedAt) < priceCacheTTL {
		return cached, nil
	}

	rows, err := s.queries.ListModelPricesForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("loading model prices: %w", err)
	}

	compiled := make([]compiledPrice, 0, len(rows))
	for _, row := range rows {
		p := compiledPrice{
			modelName:   row.ModelName,
			inputPrice:  row.InputPrice.Float64,
			outputPrice: row.OutputPrice.Float64,
			totalPrice:  row.TotalPrice.Float64,
			hasInput:    row.InputPrice.Valid,
			hasOutput:   row.OutputPrice.Valid,
			hasTotal:    row.TotalPrice.Valid,
		}
		// An unparseable pattern must not break pricing for other models.
		if re, err := regexp.Compile(row.MatchPattern); err == nil {
			p.pattern = re
		}
		compiled = append(compiled, p)
	}

	s.mu.Lock()
	s.cache[projectID] = compiled
	s.loaded[projectID] = time.Now()
	s.mu.Unlock()

	return compiled, nil
}

// Invalidate drops a project's cached prices after its pricing is edited.
func (s *CostService) Invalidate(projectID string) {
	s.mu.Lock()
	delete(s.cache, projectID)
	delete(s.loaded, projectID)
	s.mu.Unlock()
}

// ListModelPrices returns the price rows visible to a project, including the
// global defaults when projectID is empty.
func (s *CostService) ListModelPrices(ctx context.Context, projectID string) ([]db.ModelPrice, error) {
	rows, err := s.queries.ListModelPricesForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("listing model prices: %w", err)
	}
	return rows, nil
}

// CreateModelPriceRequest is the request body for defining a model price.
type CreateModelPriceRequest struct {
	ModelName    string   `json:"model_name"`
	MatchPattern string   `json:"match_pattern"`
	Unit         string   `json:"unit"`
	InputPrice   *float64 `json:"input_price"`
	OutputPrice  *float64 `json:"output_price"`
	TotalPrice   *float64 `json:"total_price"`
	Currency     string   `json:"currency"`
}

// CreateModelPrice defines a project-scoped model price.
func (s *CostService) CreateModelPrice(ctx context.Context, projectID string, req CreateModelPriceRequest) (db.ModelPrice, error) {
	if req.ModelName == "" {
		return db.ModelPrice{}, fmt.Errorf("model_name is required")
	}
	if req.InputPrice == nil && req.OutputPrice == nil && req.TotalPrice == nil {
		return db.ModelPrice{}, fmt.Errorf("at least one of input_price, output_price or total_price is required")
	}
	if req.MatchPattern == "" {
		req.MatchPattern = "(?i)^" + regexp.QuoteMeta(req.ModelName) + "$"
	}
	if _, err := regexp.Compile(req.MatchPattern); err != nil {
		return db.ModelPrice{}, fmt.Errorf("invalid match_pattern: %w", err)
	}
	if req.Unit == "" {
		req.Unit = "TOKENS"
	}
	if req.Currency == "" {
		req.Currency = "USD"
	}

	price, err := s.queries.CreateModelPrice(ctx, db.CreateModelPriceParams{
		ProjectID:    projectID,
		ModelName:    req.ModelName,
		MatchPattern: req.MatchPattern,
		Unit:         req.Unit,
		InputPrice:   nullableFloat(req.InputPrice),
		OutputPrice:  nullableFloat(req.OutputPrice),
		TotalPrice:   nullableFloat(req.TotalPrice),
		Currency:     req.Currency,
	})
	if err != nil {
		return db.ModelPrice{}, fmt.Errorf("creating model price: %w", err)
	}

	s.Invalidate(projectID)
	return price, nil
}

// DeleteModelPrice removes a project-scoped model price. Global defaults are
// not deletable through the project API.
func (s *CostService) DeleteModelPrice(ctx context.Context, projectID, id string) error {
	err := s.queries.DeleteModelPrice(ctx, db.DeleteModelPriceParams{
		ID:        id,
		ProjectID: projectID,
	})
	if err != nil {
		return fmt.Errorf("deleting model price: %w", err)
	}
	s.Invalidate(projectID)
	return nil
}

// --- usage normalization --------------------------------------------------

// usageAliases maps the token field names used by the Langfuse SDKs, the
// OpenAI-style payloads they wrap, and this project's own schema onto the
// normalized input/output/total triple.
var usageAliases = map[string][]string{
	"input":  {"input_tokens", "input", "promptTokens", "prompt_tokens", "inputTokens"},
	"output": {"output_tokens", "output", "completionTokens", "completion_tokens", "outputTokens"},
	"total":  {"total_tokens", "total", "totalTokens"},
}

// NormalizeUsage converts any accepted token-usage payload into the stored
// shape. It returns the normalized JSON and the parsed counts. A nil or
// unrecognised payload yields a zero usage and nil JSON, never an error, so a
// malformed usage field cannot reject an otherwise valid observation.
func NormalizeUsage(raw json.RawMessage) (json.RawMessage, TokenUsage) {
	if len(raw) == 0 {
		return nil, TokenUsage{}
	}

	var fields map[string]interface{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, TokenUsage{}
	}

	usage := TokenUsage{
		InputTokens:  pickInt(fields, usageAliases["input"]),
		OutputTokens: pickInt(fields, usageAliases["output"]),
		TotalTokens:  pickInt(fields, usageAliases["total"]),
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.TotalTokens == 0 {
		return nil, TokenUsage{}
	}

	encoded, err := json.Marshal(usage)
	if err != nil {
		return nil, usage
	}
	return encoded, usage
}

// parseTokenUsage reads the stored token-usage shape.
func parseTokenUsage(raw json.RawMessage) TokenUsage {
	_, usage := NormalizeUsage(raw)
	return usage
}

// pickInt returns the first key present in fields, coerced to an integer.
func pickInt(fields map[string]interface{}, keys []string) int64 {
	for _, key := range keys {
		v, ok := fields[key]
		if !ok || v == nil {
			continue
		}
		switch n := v.(type) {
		case float64:
			return int64(n)
		case int64:
			return n
		case int:
			return int64(n)
		}
	}
	return 0
}

// float8 converts a float to a Float8 for query parameters.
func float8(v float64) pgtype.Float8 {
	return pgtype.Float8{Float64: v, Valid: true}
}
