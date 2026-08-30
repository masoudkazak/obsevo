package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/obsevo/obsevo/internal/services"
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
	projectID, ok := requireProject(w, r)
	if !ok {
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
		writeServiceError(w, err, "trace not found")
		return
	}

	writeJSON(w, http.StatusCreated, trace)
}

// GetTrace handles GET /api/traces/:id.
func (h *TraceHandler) GetTrace(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	trace, observations, err := h.traceService.GetTraceDetail(r.Context(), projectID, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "trace not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"trace":        trace,
		"observations": observations,
	})
}

// DeleteTrace handles DELETE /api/traces/:id.
func (h *TraceHandler) DeleteTrace(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	if err := h.traceService.DeleteTrace(r.Context(), projectID, chi.URLParam(r, "id")); err != nil {
		writeServiceError(w, err, "trace not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListTraces handles GET /api/traces.
func (h *TraceHandler) ListTraces(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	resp, err := h.traceService.ListTraces(r.Context(), services.ListTracesRequest{
		ProjectID:   projectID,
		Name:        q.Get("name"),
		UserID:      q.Get("user_id"),
		SessionID:   q.Get("session_id"),
		Release:     q.Get("release"),
		Version:     q.Get("version"),
		Environment: q.Get("environment"),
		Tags:        queryCSV(r, "tags"),
		FromTime:    q.Get("from_time"),
		ToTime:      q.Get("to_time"),
		Limit:       queryInt32(r, "limit", 50),
		Offset:      queryInt32(r, "offset", 0),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list traces")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// CreateObservation handles POST /api/observations.
func (h *TraceHandler) CreateObservation(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	var req services.CreateObservationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TraceID == "" {
		writeError(w, http.StatusBadRequest, "trace_id is required")
		return
	}

	obs, err := h.traceService.CreateObservation(r.Context(), projectID, req)
	if err != nil {
		writeServiceError(w, err, "observation not found")
		return
	}

	writeJSON(w, http.StatusCreated, obs)
}

// GetObservation handles GET /api/observations/:id.
func (h *TraceHandler) GetObservation(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	obs, err := h.traceService.GetObservation(r.Context(), projectID, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "observation not found")
		return
	}

	writeJSON(w, http.StatusOK, obs)
}

// ListObservations handles GET /api/observations.
func (h *TraceHandler) ListObservations(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	resp, err := h.traceService.ListObservations(r.Context(), services.ListObservationsRequest{
		ProjectID: projectID,
		TraceID:   q.Get("trace_id"),
		Type:      q.Get("type"),
		Name:      q.Get("name"),
		Model:     q.Get("model"),
		Level:     q.Get("level"),
		FromTime:  q.Get("from_time"),
		ToTime:    q.Get("to_time"),
		Limit:     queryInt32(r, "limit", 50),
		Offset:    queryInt32(r, "offset", 0),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list observations")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// ListSessions handles GET /api/sessions.
func (h *TraceHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	resp, err := h.traceService.ListSessions(r.Context(), projectID,
		queryInt32(r, "limit", 50), queryInt32(r, "offset", 0))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetSession handles GET /api/sessions/:id.
func (h *TraceHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	session, traces, err := h.traceService.GetSessionDetail(r.Context(), projectID, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"session": session,
		"traces":  traces,
	})
}

// BatchIngestionRequest is the request body for batch ingestion.
type BatchIngestionRequest struct {
	Traces       []services.CreateTraceRequest       `json:"traces"`
	Observations []services.CreateObservationRequest `json:"observations"`
}

// BatchIngestion handles POST /api/ingestion.
func (h *TraceHandler) BatchIngestion(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
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
		if _, err := h.traceService.CreateTrace(r.Context(), projectID, t); err != nil {
			writeServiceError(w, err, "trace not found")
			return
		}
		createdTraces++
	}

	var createdObservations int
	for _, obs := range req.Observations {
		if obs.TraceID == "" {
			continue
		}
		if _, err := h.traceService.CreateObservation(r.Context(), projectID, obs); err != nil {
			writeServiceError(w, err, "observation not found")
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
	r.Delete("/traces/{id}", h.DeleteTrace)
	r.Post("/observations", h.CreateObservation)
	r.Get("/observations", h.ListObservations)
	r.Get("/observations/{id}", h.GetObservation)
	r.Get("/sessions", h.ListSessions)
	r.Get("/sessions/{id}", h.GetSession)
	r.Post("/ingestion", h.BatchIngestion)
}
