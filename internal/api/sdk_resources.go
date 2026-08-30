package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/langfuse-light/langfuse-light/internal/auth"
	"github.com/langfuse-light/langfuse-light/internal/services"
)

// This file carries the Langfuse-compatible prompt and dataset endpoints under
// /api/public. They are separated from sdk.go, which owns ingestion and the
// trace/observation/score read API.

// --- prompts --------------------------------------------------------------

// PublicPrompt is the prompt shape the Langfuse SDKs expect.
type PublicPrompt struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Version       int32           `json:"version"`
	Type          string          `json:"type"`
	Prompt        json.RawMessage `json:"prompt"`
	Config        json.RawMessage `json:"config"`
	Labels        []string        `json:"labels"`
	Tags          []string        `json:"tags"`
	CommitMessage string          `json:"commitMessage,omitempty"`
	Variables     []string        `json:"variables"`
	CreatedAt     string          `json:"createdAt"`
}

// GetPrompt handles GET /api/public/v2/prompts/{promptName}.
// The version is chosen by `?version=` or `?label=`, defaulting to production.
func (h *SDKHandler) GetPrompt(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	prompt, err := h.promptService.GetPrompt(r.Context(), projectID,
		chi.URLParam(r, "promptName"),
		queryInt32(r, "version", 0),
		r.URL.Query().Get("label"))
	if err != nil {
		writeError(w, http.StatusNotFound, "prompt not found")
		return
	}

	described := h.promptService.Describe(prompt)
	writeJSON(w, http.StatusOK, PublicPrompt{
		ID:            prompt.ID,
		Name:          prompt.Name,
		Version:       prompt.Version,
		Type:          prompt.Type,
		Prompt:        prompt.Prompt,
		Config:        prompt.Config,
		Labels:        prompt.Labels,
		Tags:          prompt.Tags,
		CommitMessage: prompt.CommitMessage.String,
		Variables:     described.Variables,
		CreatedAt:     prompt.CreatedAt.Time.Format(timeRFC3339),
	})
}

// ListPrompts handles GET /api/public/v2/prompts.
func (h *SDKHandler) ListPrompts(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	resp, err := h.promptService.ListPrompts(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list prompts")
		return
	}

	writeJSON(w, http.StatusOK, langfusePage(resp.Names, int64(len(resp.Names)), int32(len(resp.Names)), 0))
}

// CreatePromptRequestPublic is the Langfuse prompt-creation body.
type CreatePromptRequestPublic struct {
	Name          string          `json:"name"`
	Prompt        json.RawMessage `json:"prompt"`
	Type          string          `json:"type"`
	Config        json.RawMessage `json:"config"`
	Labels        []string        `json:"labels"`
	Tags          []string        `json:"tags"`
	CommitMessage string          `json:"commitMessage"`
}

// CreatePrompt handles POST /api/public/v2/prompts.
//
// Langfuse has one endpoint for both creating a prompt and adding a version to
// it, distinguished only by whether the name already exists, so this creates
// version 1 when the prompt is new and appends a version when it is not.
func (h *SDKHandler) CreatePrompt(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	var req CreatePromptRequestPublic
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

	existing, err := h.promptService.GetPromptVersions(r.Context(), projectID, req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to look up prompt")
		return
	}

	var (
		prompt    interface{}
		createErr error
	)
	if len(existing) == 0 {
		prompt, createErr = h.promptService.CreatePrompt(r.Context(), projectID, services.CreatePromptRequest{
			Name:          req.Name,
			Prompt:        req.Prompt,
			Config:        req.Config,
			Type:          req.Type,
			Labels:        req.Labels,
			Tags:          req.Tags,
			CommitMessage: req.CommitMessage,
		})
	} else {
		prompt, createErr = h.promptService.CreatePromptVersion(r.Context(), projectID, req.Name, services.CreatePromptVersionRequest{
			Prompt:        req.Prompt,
			Config:        req.Config,
			Type:          req.Type,
			Labels:        req.Labels,
			Tags:          req.Tags,
			CommitMessage: req.CommitMessage,
		})
	}
	if createErr != nil {
		writeError(w, http.StatusBadRequest, createErr.Error())
		return
	}

	writeJSON(w, http.StatusCreated, prompt)
}

// --- datasets -------------------------------------------------------------

// GetDataset handles GET /api/public/datasets/{datasetName}.
func (h *SDKHandler) GetDataset(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	dataset, err := h.datasetService.GetDatasetByName(r.Context(), projectID, chi.URLParam(r, "datasetName"))
	if err != nil {
		writeError(w, http.StatusNotFound, "dataset not found")
		return
	}

	items, err := h.datasetService.ListDatasetItems(r.Context(), projectID, dataset.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list dataset items")
		return
	}

	payload := toPublicDataset(dataset)
	payload.Items = toPublicDatasetItems(items, dataset.Name)
	writeJSON(w, http.StatusOK, payload)
}

// ListDatasets handles GET /api/public/datasets.
func (h *SDKHandler) ListDatasets(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	datasets, err := h.datasetService.ListDatasets(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list datasets")
		return
	}

	writeJSON(w, http.StatusOK, langfusePage(toPublicDatasets(datasets), int64(len(datasets)), int32(len(datasets)), 0))
}

// CreateDatasetRequestPublic is the Langfuse dataset-creation body.
type CreateDatasetRequestPublic struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CreateDataset handles POST /api/public/datasets.
func (h *SDKHandler) CreateDataset(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	var req CreateDatasetRequestPublic
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Langfuse treats dataset creation as idempotent on the name.
	if existing, err := h.datasetService.GetDatasetByName(r.Context(), projectID, req.Name); err == nil {
		writeJSON(w, http.StatusOK, toPublicDataset(existing))
		return
	}

	dataset, err := h.datasetService.CreateDataset(r.Context(), projectID, services.CreateDatasetRequest{
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, toPublicDataset(dataset))
}

// CreateDatasetItemRequestPublic is the Langfuse dataset-item body.
type CreateDatasetItemRequestPublic struct {
	ID                  string          `json:"id"`
	DatasetName         string          `json:"datasetName"`
	Input               json.RawMessage `json:"input"`
	ExpectedOutput      json.RawMessage `json:"expectedOutput"`
	Metadata            json.RawMessage `json:"metadata"`
	SourceTraceID       string          `json:"sourceTraceId"`
	SourceObservationID string          `json:"sourceObservationId"`
	Status              string          `json:"status"`
}

// CreateDatasetItem handles POST /api/public/dataset-items.
func (h *SDKHandler) CreateDatasetItem(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	var req CreateDatasetItemRequestPublic
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	dataset, err := h.datasetService.GetDatasetByName(r.Context(), projectID, req.DatasetName)
	if err != nil {
		writeError(w, http.StatusNotFound, "dataset not found")
		return
	}

	item, err := h.datasetService.CreateDatasetItem(r.Context(), projectID, dataset.ID, services.CreateDatasetItemRequest{
		ID:                  req.ID,
		Input:               req.Input,
		ExpectedOutput:      req.ExpectedOutput,
		Metadata:            req.Metadata,
		SourceTraceID:       req.SourceTraceID,
		SourceObservationID: req.SourceObservationID,
		Status:              req.Status,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, toPublicDatasetItem(item, dataset.Name))
}

// CreateDatasetRunItemRequestPublic is the Langfuse dataset-run-item body.
// The run is addressed by name and created on first use, which is what lets an
// experiment script open a run simply by naming it.
type CreateDatasetRunItemRequestPublic struct {
	RunName        string          `json:"runName"`
	RunDescription string          `json:"runDescription"`
	Metadata       json.RawMessage `json:"metadata"`
	DatasetItemID  string          `json:"datasetItemId"`
	TraceID        string          `json:"traceId"`
	ObservationID  string          `json:"observationId"`
}

// CreateDatasetRunItem handles POST /api/public/dataset-run-items.
func (h *SDKHandler) CreateDatasetRunItem(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	var req CreateDatasetRunItemRequestPublic
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DatasetItemID == "" || req.RunName == "" {
		writeError(w, http.StatusBadRequest, "datasetItemId and runName are required")
		return
	}

	run, err := h.datasetService.EnsureRunByName(r.Context(), projectID, req.DatasetItemID, services.EnsureRunRequest{
		Name:        req.RunName,
		Description: req.RunDescription,
		Metadata:    req.Metadata,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	runItem, err := h.datasetService.CreateDatasetRunItem(r.Context(), projectID, run.DatasetID, run.ID,
		services.CreateDatasetRunItemRequest{
			DatasetItemID: req.DatasetItemID,
			TraceID:       req.TraceID,
			ObservationID: req.ObservationID,
		})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, runItem)
}

// RegisterResourceRoutes registers the public prompt and dataset routes.
func (h *SDKHandler) RegisterResourceRoutes(r chi.Router) {
	r.Get("/v2/prompts", h.ListPrompts)
	r.Post("/v2/prompts", h.CreatePrompt)
	r.Get("/v2/prompts/{promptName}", h.GetPrompt)

	r.Get("/datasets", h.ListDatasets)
	r.Post("/datasets", h.CreateDataset)
	r.Get("/datasets/{datasetName}", h.GetDataset)
	r.Post("/dataset-items", h.CreateDatasetItem)
	r.Post("/dataset-run-items", h.CreateDatasetRunItem)
}

// timeRFC3339 is the timestamp layout used across the public API.
const timeRFC3339 = "2006-01-02T15:04:05Z07:00"
