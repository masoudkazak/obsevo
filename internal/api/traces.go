package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/langfuse-light/langfuse-light/internal/services"
)

// TraceHandler handles trace and observation HTTP requests.
type TraceHandler struct {
	traceService *services.TraceService
}

// NewTraceHandler creates a new trace handler.
func NewTraceHandler(traceService *services.TraceService) *TraceHandler {
	return &TraceHandler{traceService: traceService}
}

// CreateTrace handles POST /api/traces.
func (h *TraceHandler) CreateTrace(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	var req services.CreateTraceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	trace, err := h.traceService.CreateTrace(r.Context(), projectID, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create trace: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, trace)
}

// GetTrace handles GET /api/traces/:id.
func (h *TraceHandler) GetTrace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	trace, observations, err := h.traceService.GetTraceDetail(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "trace not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"trace":        trace,
		"observations": observations,
	})
}

// ListTraces handles GET /api/traces.
func (h *TraceHandler) ListTraces(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	limit, _ := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 32)
	offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 32)
	name := r.URL.Query().Get("name")

	resp, err := h.traceService.ListTraces(r.Context(), services.ListTracesRequest{
		ProjectID: projectID,
		Name:      name,
		Limit:     int32(limit),
		Offset:    int32(offset),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list traces")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// CreateObservation handles POST /api/observations.
func (h *TraceHandler) CreateObservation(w http.ResponseWriter, r *http.Request) {
	var req services.CreateObservationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TraceID == "" {
		writeError(w, http.StatusBadRequest, "trace_id is required")
		return
	}

	if req.Type == "" {
		req.Type = "SPAN"
	}

	obs, err := h.traceService.CreateObservation(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create observation: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, obs)
}

// GetObservation handles GET /api/observations/:id.
func (h *TraceHandler) GetObservation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	obs, err := h.traceService.GetObservation(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "observation not found")
		return
	}

	writeJSON(w, http.StatusOK, obs)
}

// BatchIngestionRequest is the request body for batch ingestion.
type BatchIngestionRequest struct {
	Traces       []services.CreateTraceRequest       `json:"traces"`
	Observations []services.CreateObservationRequest `json:"observations"`
}

// BatchIngestion handles POST /api/ingestion.
func (h *TraceHandler) BatchIngestion(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	var req BatchIngestionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var createdTraces int
	for _, t := range req.Traces {
		if t.ID == "" {
			continue
		}
		_, err := h.traceService.CreateTrace(r.Context(), projectID, t)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create trace: "+err.Error())
			return
		}
		createdTraces++
	}

	var createdObservations int
	for _, obs := range req.Observations {
		if obs.TraceID == "" {
			continue
		}
		_, err := h.traceService.CreateObservation(r.Context(), obs)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create observation: "+err.Error())
			return
		}
		createdObservations++
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"traces_created":       createdTraces,
		"observations_created": createdObservations,
	})
}

// RegisterRoutes registers trace routes.
func (h *TraceHandler) RegisterRoutes(r chi.Router) {
	r.Post("/traces", h.CreateTrace)
	r.Get("/traces", h.ListTraces)
	r.Get("/traces/{id}", h.GetTrace)
	r.Post("/observations", h.CreateObservation)
	r.Get("/observations/{id}", h.GetObservation)
	r.Post("/ingestion", h.BatchIngestion)
}
