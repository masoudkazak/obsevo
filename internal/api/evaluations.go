package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/obsevo/obsevo/internal/auth"
	"github.com/obsevo/obsevo/internal/services"
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
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	var req services.CreateScoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	score, err := h.evalService.CreateScore(r.Context(), projectID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, score)
}

// GetScore handles GET /api/scores/:id.
func (h *EvaluationHandler) GetScore(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	score, err := h.evalService.GetScore(r.Context(), projectID, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "score not found")
		return
	}

	writeJSON(w, http.StatusOK, score)
}

// DeleteScore handles DELETE /api/scores/:id.
func (h *EvaluationHandler) DeleteScore(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	if err := h.evalService.DeleteScore(r.Context(), projectID, chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete score")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListScores handles GET /api/scores.
func (h *EvaluationHandler) ListScores(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	resp, err := h.evalService.ListScores(r.Context(), services.ListScoresRequest{
		ProjectID:     projectID,
		Name:          q.Get("name"),
		Source:        q.Get("source"),
		TraceID:       q.Get("trace_id"),
		ObservationID: q.Get("observation_id"),
		DataType:      q.Get("data_type"),
		Limit:         queryInt32(r, "limit", 50),
		Offset:        queryInt32(r, "offset", 0),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list scores")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// ListTraceScores handles GET /api/traces/:id/scores.
func (h *EvaluationHandler) ListTraceScores(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	resp, err := h.evalService.ListScores(r.Context(), services.ListScoresRequest{
		ProjectID: projectID,
		TraceID:   chi.URLParam(r, "id"),
		Name:      r.URL.Query().Get("name"),
		Limit:     queryInt32(r, "limit", 200),
		Offset:    queryInt32(r, "offset", 0),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list scores: "+err.Error())
		return
	}

	// Historically this endpoint returned a bare array; keep that shape so the
	// existing dashboard and any SDK consumers are unaffected.
	writeJSON(w, http.StatusOK, resp.Scores)
}

// GetScoreAggregations handles GET /api/scores/aggregation.
func (h *EvaluationHandler) GetScoreAggregations(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
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
	projectID := auth.GetProjectID(r.Context())
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
	projectID := auth.GetProjectID(r.Context())
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
	projectID := auth.GetProjectID(r.Context())
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
	projectID := auth.GetProjectID(r.Context())
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
	projectID := auth.GetProjectID(r.Context())
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
	projectID := auth.GetProjectID(r.Context())
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
	projectID := auth.GetProjectID(r.Context())
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
	projectID := auth.GetProjectID(r.Context())
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
	projectID := auth.GetProjectID(r.Context())
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

// analyticsFilter builds the shared analytics filter from query parameters.
func analyticsFilter(r *http.Request, projectID string) services.AnalyticsFilter {
	q := r.URL.Query()
	return services.AnalyticsFilter{
		ProjectID:   projectID,
		UserID:      q.Get("user_id"),
		SessionID:   q.Get("session_id"),
		Environment: q.Get("environment"),
		Tags:        queryCSV(r, "tags"),
		FromTime:    q.Get("from_time"),
		ToTime:      q.Get("to_time"),
	}
}

// GetDashboard handles GET /api/analytics/dashboard — every headline metric in
// one request, so the dashboard does not fan out into a dozen calls.
func (h *EvaluationHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	summary, err := h.evalService.GetDashboardSummary(r.Context(), analyticsFilter(r, projectID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build dashboard: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, summary)
}

// GetLatencyPercentiles handles GET /api/analytics/percentiles.
func (h *EvaluationHandler) GetLatencyPercentiles(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	stats, err := h.evalService.GetLatencyPercentiles(r.Context(), analyticsFilter(r, projectID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get latency percentiles")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// GetModelUsage handles GET /api/analytics/models.
func (h *EvaluationHandler) GetModelUsage(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	usage, err := h.evalService.GetModelUsage(r.Context(), analyticsFilter(r, projectID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get model usage")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"models": usage})
}

// GetCostByUser handles GET /api/analytics/users.
func (h *EvaluationHandler) GetCostByUser(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	costs, err := h.evalService.GetCostByUser(r.Context(), analyticsFilter(r, projectID), queryInt32(r, "limit", 50))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get cost by user")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"users": costs})
}

// GetMetricsOverTime handles GET /api/analytics/timeseries.
func (h *EvaluationHandler) GetMetricsOverTime(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	series, err := h.evalService.GetMetricsOverTime(r.Context(),
		analyticsFilter(r, projectID), r.URL.Query().Get("bucket"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, series)
}

// RegisterRoutes registers evaluation routes.
func (h *EvaluationHandler) RegisterRoutes(r chi.Router) {
	r.Post("/scores", h.CreateScore)
	r.Get("/scores", h.ListScores)
	r.Get("/scores/aggregation", h.GetScoreAggregations)
	r.Get("/scores/{id}", h.GetScore)
	r.Delete("/scores/{id}", h.DeleteScore)
	r.Get("/traces/{id}/scores", h.ListTraceScores)
	r.Get("/analytics", h.GetAnalytics)
	r.Get("/analytics/dashboard", h.GetDashboard)
	r.Get("/analytics/percentiles", h.GetLatencyPercentiles)
	r.Get("/analytics/models", h.GetModelUsage)
	r.Get("/analytics/users", h.GetCostByUser)
	r.Get("/analytics/timeseries", h.GetMetricsOverTime)
	r.Get("/analytics/latency", h.GetLatencyStats)
	r.Get("/analytics/cost", h.GetCostStats)
	r.Get("/analytics/tokens", h.GetTokenUsageStats)
	r.Get("/analytics/errors", h.GetErrorRateStats)
	r.Get("/analytics/cost-over-time", h.GetCostOverTime)
	r.Get("/analytics/latency-over-time", h.GetLatencyOverTime)
	r.Get("/analytics/tokens-over-time", h.GetTokenUsageOverTime)
	r.Get("/analytics/traces-over-time", h.GetTraceCountOverTime)
}
