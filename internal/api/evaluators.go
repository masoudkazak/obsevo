package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/langfuse-light/langfuse-light/internal/auth"
	"github.com/langfuse-light/langfuse-light/internal/services"
)

// EvaluatorHandler handles evaluator HTTP requests.
type EvaluatorHandler struct {
	evaluatorService *services.EvaluatorConfigService
}

// NewEvaluatorHandler creates a new evaluator handler.
func NewEvaluatorHandler(evaluatorService *services.EvaluatorConfigService) *EvaluatorHandler {
	return &EvaluatorHandler{evaluatorService: evaluatorService}
}

// CreateEvaluatorConfig handles POST /api/evaluators.
func (h *EvaluatorHandler) CreateEvaluatorConfig(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	var req services.CreateEvaluatorConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	config, err := h.evaluatorService.CreateEvaluatorConfig(r.Context(), projectID, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create evaluator config: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, config)
}

// ListEvaluatorConfigs handles GET /api/evaluators.
func (h *EvaluatorHandler) ListEvaluatorConfigs(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	configs, err := h.evaluatorService.ListEvaluatorConfigs(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list evaluator configs: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, configs)
}

// GetEvaluatorConfig handles GET /api/evaluators/{id}.
func (h *EvaluatorHandler) GetEvaluatorConfig(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	config, err := h.evaluatorService.GetEvaluatorConfig(r.Context(), projectID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "evaluator config not found")
		return
	}

	writeJSON(w, http.StatusOK, config)
}

// DeleteEvaluatorConfig handles DELETE /api/evaluators/{id}.
func (h *EvaluatorHandler) DeleteEvaluatorConfig(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	if err := h.evaluatorService.DeleteEvaluatorConfig(r.Context(), projectID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete evaluator config: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// RunEvaluation handles POST /api/evaluators/{id}/run.
func (h *EvaluatorHandler) RunEvaluation(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}
	evaluatorID := chi.URLParam(r, "id")

	var req services.EvaluateTracesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.TraceIDs) == 0 {
		writeError(w, http.StatusBadRequest, "trace_ids is required and must not be empty")
		return
	}

	run, results, err := h.evaluatorService.EvaluateTraces(r.Context(), projectID, evaluatorID, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to run evaluation: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"run":     run,
		"results": results,
	})
}

// ListEvaluationRuns handles GET /api/evaluators/runs.
func (h *EvaluatorHandler) ListEvaluationRuns(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	runs, err := h.evaluatorService.ListEvaluationRuns(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list evaluation runs: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, runs)
}

// GetEvaluationRun handles GET /api/evaluators/runs/{id}.
func (h *EvaluatorHandler) GetEvaluationRun(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	run, err := h.evaluatorService.GetEvaluationRun(r.Context(), projectID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "evaluation run not found")
		return
	}

	writeJSON(w, http.StatusOK, run)
}

// ListEvaluatorTypes handles GET /api/evaluators/types.
// It reports every registered evaluator, so a client never has to hardcode
// the list.
func (h *EvaluatorHandler) ListEvaluatorTypes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"types": services.AvailableEvaluators(),
	})
}

// RegisterRoutes registers evaluator routes.
func (h *EvaluatorHandler) RegisterRoutes(r chi.Router) {
	r.Post("/evaluators", h.CreateEvaluatorConfig)
	r.Get("/evaluators", h.ListEvaluatorConfigs)
	r.Get("/evaluators/types", h.ListEvaluatorTypes)
	r.Get("/evaluators/{id}", h.GetEvaluatorConfig)
	r.Delete("/evaluators/{id}", h.DeleteEvaluatorConfig)
	r.Post("/evaluators/{id}/run", h.RunEvaluation)
	r.Get("/evaluators/runs", h.ListEvaluationRuns)
	r.Get("/evaluators/runs/{id}", h.GetEvaluationRun)
}
