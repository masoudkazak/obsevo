package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/langfuse-light/langfuse-light/internal/auth"
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
	projectID, ok := requireProject(w, r)
	if !ok {
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
	if len(req.Prompt) == 0 {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	if req.CreatedBy == "" {
		req.CreatedBy = auth.GetUserID(r.Context())
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
	projectID, ok := requireProject(w, r)
	if !ok {
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
// The served version is selected by `?version=` or `?label=`, defaulting to the
// production label.
func (h *PromptHandler) GetPromptByName(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	prompt, err := h.promptService.GetPrompt(r.Context(), projectID,
		chi.URLParam(r, "name"),
		queryInt32(r, "version", 0),
		r.URL.Query().Get("label"))
	if err != nil {
		writeError(w, http.StatusNotFound, "prompt not found")
		return
	}

	writeJSON(w, http.StatusOK, h.promptService.Describe(prompt))
}

// CreatePromptVersion handles PUT /api/prompts/:name.
func (h *PromptHandler) CreatePromptVersion(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	var req services.CreatePromptVersionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Prompt) == 0 {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	if req.CreatedBy == "" {
		req.CreatedBy = auth.GetUserID(r.Context())
	}

	prompt, err := h.promptService.CreatePromptVersion(r.Context(), projectID, chi.URLParam(r, "name"), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, prompt)
}

// GetPromptVersions handles GET /api/prompts/:name/versions.
func (h *PromptHandler) GetPromptVersions(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	versions, err := h.promptService.GetPromptVersions(r.Context(), projectID, chi.URLParam(r, "name"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get prompt versions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"versions": versions})
}

// SetPromptActive handles POST /api/prompts/:name/active.
// It points the production label at the given version, which is also how a
// rollback is performed.
func (h *PromptHandler) SetPromptActive(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	var req services.SetPromptActiveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Version <= 0 {
		writeError(w, http.StatusBadRequest, "version must be positive")
		return
	}

	if err := h.promptService.SetPromptActive(r.Context(), projectID, chi.URLParam(r, "name"), req.Version); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// SetPromptLabels handles POST /api/prompts/:name/labels.
func (h *PromptHandler) SetPromptLabels(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	var req services.SetPromptLabelsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.promptService.SetPromptLabels(r.Context(), projectID, chi.URLParam(r, "name"), req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GetPromptByVersion handles GET /api/prompts/:name/versions/:version.
func (h *PromptHandler) GetPromptByVersion(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	version, err := strconv.ParseInt(chi.URLParam(r, "version"), 10, 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid version number")
		return
	}

	prompt, err := h.promptService.GetPromptByVersion(r.Context(), projectID, chi.URLParam(r, "name"), int32(version))
	if err != nil {
		writeError(w, http.StatusNotFound, "prompt version not found")
		return
	}

	writeJSON(w, http.StatusOK, prompt)
}

// DeletePromptVersion handles DELETE /api/prompts/:name/versions/:version.
func (h *PromptHandler) DeletePromptVersion(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	version, err := strconv.ParseInt(chi.URLParam(r, "version"), 10, 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid version number")
		return
	}

	if err := h.promptService.DeletePromptVersion(r.Context(), projectID, chi.URLParam(r, "name"), int32(version)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete prompt version")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// DeletePrompt handles DELETE /api/prompts/:name.
func (h *PromptHandler) DeletePrompt(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	if err := h.promptService.DeletePrompt(r.Context(), projectID, chi.URLParam(r, "name")); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete prompt")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// CompilePromptRequest is the request body for compiling a prompt template.
type CompilePromptRequest struct {
	Version   int32             `json:"version"`
	Label     string            `json:"label"`
	Variables map[string]string `json:"variables"`
}

// CompilePrompt handles POST /api/prompts/:name/compile — substitutes variables
// into the resolved version and returns the result.
func (h *PromptHandler) CompilePrompt(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	var req CompilePromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	prompt, err := h.promptService.GetPrompt(r.Context(), projectID, chi.URLParam(r, "name"), req.Version, req.Label)
	if err != nil {
		writeError(w, http.StatusNotFound, "prompt not found")
		return
	}

	described := h.promptService.Describe(prompt)
	compiled, err := h.promptService.CompileTemplate(described.PromptText, req.Variables)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	described.CompiledTemplate = compiled
	writeJSON(w, http.StatusOK, described)
}

// RegisterRoutes registers prompt routes.
func (h *PromptHandler) RegisterRoutes(r chi.Router) {
	r.Post("/prompts", h.CreatePrompt)
	r.Get("/prompts", h.ListPrompts)
	r.Get("/prompts/{name}", h.GetPromptByName)
	r.Put("/prompts/{name}", h.CreatePromptVersion)
	r.Delete("/prompts/{name}", h.DeletePrompt)
	r.Get("/prompts/{name}/versions", h.GetPromptVersions)
	r.Get("/prompts/{name}/versions/{version}", h.GetPromptByVersion)
	r.Delete("/prompts/{name}/versions/{version}", h.DeletePromptVersion)
	r.Post("/prompts/{name}/active", h.SetPromptActive)
	r.Post("/prompts/{name}/labels", h.SetPromptLabels)
	r.Post("/prompts/{name}/compile", h.CompilePrompt)
}
