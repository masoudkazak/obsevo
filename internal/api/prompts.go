package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/langfuse-light/langfuse-light/internal/services"
)

// PromptHandler handles prompt HTTP requests.
type PromptHandler struct {
	promptService *services.PromptService
}

// NewPromptHandler creates a new prompt handler.
func NewPromptHandler(promptService *services.PromptService) *PromptHandler {
	return &PromptHandler{promptService: promptService}
}

// CreatePrompt handles POST /api/prompts.
func (h *PromptHandler) CreatePrompt(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	var req services.CreatePromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}

	prompt, err := h.promptService.CreatePrompt(r.Context(), projectID, req)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, prompt)
}

// ListPrompts handles GET /api/prompts.
func (h *PromptHandler) ListPrompts(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	resp, err := h.promptService.ListPrompts(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list prompts")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetPromptByName handles GET /api/prompts/:name.
func (h *PromptHandler) GetPromptByName(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	name := chi.URLParam(r, "name")

	resp, err := h.promptService.GetPromptByNameWithTemplate(r.Context(), projectID, name)
	if err != nil {
		writeError(w, http.StatusNotFound, "prompt not found")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// CreatePromptVersion handles PUT /api/prompts/:name.
func (h *PromptHandler) CreatePromptVersion(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	name := chi.URLParam(r, "name")

	var req services.CreatePromptVersionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}

	prompt, err := h.promptService.CreatePromptVersion(r.Context(), projectID, name, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, prompt)
}

// GetPromptVersions handles GET /api/prompts/:name/versions.
func (h *PromptHandler) GetPromptVersions(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	name := chi.URLParam(r, "name")

	versions, err := h.promptService.GetPromptVersions(r.Context(), projectID, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get prompt versions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"versions": versions,
	})
}

// SetPromptActive handles POST /api/prompts/:name/active.
func (h *PromptHandler) SetPromptActive(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	name := chi.URLParam(r, "name")

	var req services.SetPromptActiveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Version <= 0 {
		writeError(w, http.StatusBadRequest, "version must be positive")
		return
	}

	err := h.promptService.SetPromptActive(r.Context(), projectID, name, req.Version)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GetPromptByVersion handles GET /api/prompts/:name/versions/:version.
func (h *PromptHandler) GetPromptByVersion(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	name := chi.URLParam(r, "name")
	versionStr := chi.URLParam(r, "version")

	version, err := strconv.ParseInt(versionStr, 10, 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid version number")
		return
	}

	prompt, err := h.promptService.GetPromptByVersion(r.Context(), projectID, name, int32(version))
	if err != nil {
		writeError(w, http.StatusNotFound, "prompt version not found")
		return
	}

	writeJSON(w, http.StatusOK, prompt)
}

// RegisterRoutes registers prompt routes.
func (h *PromptHandler) RegisterRoutes(r chi.Router) {
	r.Post("/prompts", h.CreatePrompt)
	r.Get("/prompts", h.ListPrompts)
	r.Get("/prompts/{name}", h.GetPromptByName)
	r.Put("/prompts/{name}", h.CreatePromptVersion)
	r.Get("/prompts/{name}/versions", h.GetPromptVersions)
	r.Post("/prompts/{name}/active", h.SetPromptActive)
	r.Get("/prompts/{name}/versions/{version}", h.GetPromptByVersion)
}
