package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/langfuse-light/langfuse-light/internal/services"
)

// EvaluationHandler handles evaluation and analytics HTTP requests.
type EvaluationHandler struct {
	evalService *services.EvaluationService
}

// NewEvaluationHandler creates a new evaluation handler.
func NewEvaluationHandler(evalService *services.EvaluationService) *EvaluationHandler {
	return &EvaluationHandler{evalService: evalService}
}

// CreateScore handles POST /api/scores.
func (h *EvaluationHandler) CreateScore(w http.ResponseWriter, r *http.Request) {
	var req services.CreateScoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TraceID == "" {
		writeError(w, http.StatusBadRequest, "trace_id is required")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	score, err := h.evalService.CreateScore(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create score: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, score)
}

// GetScore handles GET /api/scores/:id.
func (h *EvaluationHandler) GetScore(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	score, err := h.evalService.GetScore(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "score not found")
		return
	}

	writeJSON(w, http.StatusOK, score)
}

// ListTraceScores handles GET /api/traces/:id/scores.
func (h *EvaluationHandler) ListTraceScores(w http.ResponseWriter, r *http.Request) {
	traceID := chi.URLParam(r, "id")
	name := r.URL.Query().Get("name")

	var scores interface{}
	var err error

	if name != "" {
		scores, err = h.evalService.ListScoresByTraceAndName(r.Context(), traceID, name)
	} else {
		scores, err = h.evalService.ListScoresByTrace(r.Context(), traceID)
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list scores: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, scores)
}

// GetScoreAggregations handles GET /api/scores/aggregation.
func (h *EvaluationHandler) GetScoreAggregations(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	aggregations, err := h.evalService.GetScoreAggregations(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get score aggregations: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, aggregations)
}

// GetAnalytics handles GET /api/analytics.
func (h *EvaluationHandler) GetAnalytics(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	summary, err := h.evalService.GetAnalyticsSummary(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get analytics: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, summary)
}

// GetLatencyStats handles GET /api/analytics/latency.
func (h *EvaluationHandler) GetLatencyStats(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	stats, err := h.evalService.GetLatencyStats(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get latency stats: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// GetCostStats handles GET /api/analytics/cost.
func (h *EvaluationHandler) GetCostStats(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	stats, err := h.evalService.GetCostStats(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get cost stats: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// GetTokenUsageStats handles GET /api/analytics/tokens.
func (h *EvaluationHandler) GetTokenUsageStats(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	stats, err := h.evalService.GetTokenUsageStats(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get token usage stats: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// GetErrorRateStats handles GET /api/analytics/errors.
func (h *EvaluationHandler) GetErrorRateStats(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	stats, err := h.evalService.GetErrorRateStats(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get error rate stats: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// GetCostOverTime handles GET /api/analytics/cost-over-time.
func (h *EvaluationHandler) GetCostOverTime(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if v, err := strconv.Atoi(d); err == nil && v > 0 && v <= 365 {
			days = v
		}
	}

	result, err := h.evalService.GetCostOverTime(r.Context(), projectID, days)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get cost over time: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// GetLatencyOverTime handles GET /api/analytics/latency-over-time.
func (h *EvaluationHandler) GetLatencyOverTime(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if v, err := strconv.Atoi(d); err == nil && v > 0 && v <= 365 {
			days = v
		}
	}

	result, err := h.evalService.GetLatencyOverTime(r.Context(), projectID, days)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get latency over time: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// GetTokenUsageOverTime handles GET /api/analytics/tokens-over-time.
func (h *EvaluationHandler) GetTokenUsageOverTime(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if v, err := strconv.Atoi(d); err == nil && v > 0 && v <= 365 {
			days = v
		}
	}

	result, err := h.evalService.GetTokenUsageOverTime(r.Context(), projectID, days)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get token usage over time: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// GetTraceCountOverTime handles GET /api/analytics/traces-over-time.
func (h *EvaluationHandler) GetTraceCountOverTime(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if v, err := strconv.Atoi(d); err == nil && v > 0 && v <= 365 {
			days = v
		}
	}

	result, err := h.evalService.GetTraceCountOverTime(r.Context(), projectID, days)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get trace count over time: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// RegisterRoutes registers evaluation routes.
func (h *EvaluationHandler) RegisterRoutes(r chi.Router) {
	r.Post("/scores", h.CreateScore)
	r.Get("/scores/{id}", h.GetScore)
	r.Get("/scores/aggregation", h.GetScoreAggregations)
	r.Get("/traces/{id}/scores", h.ListTraceScores)
	r.Get("/analytics", h.GetAnalytics)
	r.Get("/analytics/latency", h.GetLatencyStats)
	r.Get("/analytics/cost", h.GetCostStats)
	r.Get("/analytics/tokens", h.GetTokenUsageStats)
	r.Get("/analytics/errors", h.GetErrorRateStats)
	r.Get("/analytics/cost-over-time", h.GetCostOverTime)
	r.Get("/analytics/latency-over-time", h.GetLatencyOverTime)
	r.Get("/analytics/tokens-over-time", h.GetTokenUsageOverTime)
	r.Get("/analytics/traces-over-time", h.GetTraceCountOverTime)
}
