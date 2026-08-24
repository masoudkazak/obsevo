package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/langfuse-light/langfuse-light/internal/auth"
	"github.com/langfuse-light/langfuse-light/internal/services"
)

// SDKHandler handles SDK-compatible ingestion endpoints.
type SDKHandler struct {
	traceService *services.TraceService
}

// NewSDKHandler creates a new SDK handler.
func NewSDKHandler(traceService *services.TraceService) *SDKHandler {
	return &SDKHandler{traceService: traceService}
}

// CreateTraceRequest represents the Langfuse SDK trace creation request.
type CreateTraceRequest struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	Output    json.RawMessage `json:"output"`
	Metadata  json.RawMessage `json:"metadata"`
	UserID    string          `json:"userId"`
	SessionID string          `json:"sessionId"`
	Tags      []string        `json:"tags"`
	StartTime string          `json:"startTime"`
	EndTime   string          `json:"endTime"`
}

// CreateTraceResponse represents the response for trace creation.
type CreateTraceResponse struct {
	ID string `json:"id"`
}

// CreateTrace handles POST /api/public/traces — SDK-compatible endpoint.
func (h *SDKHandler) CreateTrace(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	var req CreateTraceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	trace, err := h.traceService.CreateTrace(r.Context(), projectID, services.CreateTraceRequest{
		ID:        req.ID,
		Name:      req.Name,
		Input:     req.Input,
		Output:    req.Output,
		Metadata:  req.Metadata,
		UserID:    req.UserID,
		SessionID: req.SessionID,
		Tags:      req.Tags,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create trace: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, CreateTraceResponse{ID: trace.ID})
}

// CreateObservationRequest represents the Langfuse SDK observation creation request.
type CreateObservationRequest struct {
	ID                  string          `json:"id"`
	TraceID             string          `json:"traceId"`
	Type                string          `json:"type"`
	Name                string          `json:"name"`
	Input               json.RawMessage `json:"input"`
	Output              json.RawMessage `json:"output"`
	Metadata            json.RawMessage `json:"metadata"`
	Model               string          `json:"model"`
	ModelParameters     json.RawMessage `json:"modelParameters"`
	StartTime           string          `json:"startTime"`
	EndTime             string          `json:"endTime"`
	TokenUsage          json.RawMessage `json:"usage"`
	Cost                *float64        `json:"cost"`
	Status              string          `json:"status"`
	ParentObservationID string          `json:"parentObservationId"`
}

// CreateObservationResponse represents the response for observation creation.
type CreateObservationResponse struct {
	ID string `json:"id"`
}

// CreateObservation handles POST /api/public/observations — SDK-compatible endpoint.
func (h *SDKHandler) CreateObservation(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	var req CreateObservationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TraceID == "" {
		writeError(w, http.StatusBadRequest, "traceId is required")
		return
	}

	if req.Type == "" {
		req.Type = "SPAN"
	}

	obs, err := h.traceService.CreateObservation(r.Context(), services.CreateObservationRequest{
		ID:                  req.ID,
		TraceID:             req.TraceID,
		Type:                req.Type,
		Name:                req.Name,
		Input:               req.Input,
		Output:              req.Output,
		Metadata:            req.Metadata,
		Model:               req.Model,
		ModelParameters:     req.ModelParameters,
		StartTime:           req.StartTime,
		EndTime:             req.EndTime,
		TokenUsage:          req.TokenUsage,
		Cost:                req.Cost,
		Status:              req.Status,
		ParentObservationID: req.ParentObservationID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create observation: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, CreateObservationResponse{ID: obs.ID})
}

// BatchIngestionBody is the body for SDK batch ingestion.
type BatchIngestionBody struct {
	Batch []BatchItem `json:"batch"`
}

// BatchItem represents a single item in a batch ingestion request.
type BatchItem struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Body      json.RawMessage `json:"body"`
}

// BatchIngestionResponse represents the response for batch ingestion.
type BatchIngestionResponse struct {
	Successes []BatchResult `json:"successes"`
	Errors    []BatchResult `json:"errors"`
}

// BatchResult represents a single result in batch ingestion.
type BatchResult struct {
	ID     string `json:"id"`
	Status int    `json:"status"`
}

// BatchIngestion handles POST /api/public/ingestion — SDK-compatible batch endpoint.
func (h *SDKHandler) BatchIngestion(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	var body BatchIngestionBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	resp := BatchIngestionResponse{
		Successes: []BatchResult{},
		Errors:    []BatchResult{},
	}

	for _, item := range body.Batch {
		switch item.Type {
		case "trace-create":
			var traceReq CreateTraceRequest
			if err := json.Unmarshal(item.Body, &traceReq); err != nil {
				resp.Errors = append(resp.Errors, BatchResult{ID: item.ID, Status: http.StatusBadRequest})
				continue
			}
			if traceReq.ID == "" {
				traceReq.ID = item.ID
			}
			_, err := h.traceService.CreateTrace(r.Context(), projectID, services.CreateTraceRequest{
				ID:        traceReq.ID,
				Name:      traceReq.Name,
				Input:     traceReq.Input,
				Output:    traceReq.Output,
				Metadata:  traceReq.Metadata,
				UserID:    traceReq.UserID,
				SessionID: traceReq.SessionID,
				Tags:      traceReq.Tags,
				StartTime: traceReq.StartTime,
				EndTime:   traceReq.EndTime,
			})
			if err != nil {
				resp.Errors = append(resp.Errors, BatchResult{ID: item.ID, Status: http.StatusInternalServerError})
				continue
			}
			resp.Successes = append(resp.Successes, BatchResult{ID: item.ID, Status: http.StatusOK})

		case "trace-update":
			var traceReq CreateTraceRequest
			if err := json.Unmarshal(item.Body, &traceReq); err != nil {
				resp.Errors = append(resp.Errors, BatchResult{ID: item.ID, Status: http.StatusBadRequest})
				continue
			}
			_, err := h.traceService.CreateTrace(r.Context(), projectID, services.CreateTraceRequest{
				ID:       traceReq.ID,
				Name:     traceReq.Name,
				Input:    traceReq.Input,
				Output:   traceReq.Output,
				Metadata: traceReq.Metadata,
				EndTime:  traceReq.EndTime,
			})
			if err != nil {
				resp.Errors = append(resp.Errors, BatchResult{ID: item.ID, Status: http.StatusInternalServerError})
				continue
			}
			resp.Successes = append(resp.Successes, BatchResult{ID: item.ID, Status: http.StatusOK})

		case "observation-create":
			var obsReq CreateObservationRequest
			if err := json.Unmarshal(item.Body, &obsReq); err != nil {
				resp.Errors = append(resp.Errors, BatchResult{ID: item.ID, Status: http.StatusBadRequest})
				continue
			}
			if obsReq.Type == "" {
				obsReq.Type = "SPAN"
			}
			_, err := h.traceService.CreateObservation(r.Context(), services.CreateObservationRequest{
				ID:                  obsReq.ID,
				TraceID:             obsReq.TraceID,
				Type:                obsReq.Type,
				Name:                obsReq.Name,
				Input:               obsReq.Input,
				Output:              obsReq.Output,
				Metadata:            obsReq.Metadata,
				Model:               obsReq.Model,
				ModelParameters:     obsReq.ModelParameters,
				StartTime:           obsReq.StartTime,
				EndTime:             obsReq.EndTime,
				TokenUsage:          obsReq.TokenUsage,
				Cost:                obsReq.Cost,
				Status:              obsReq.Status,
				ParentObservationID: obsReq.ParentObservationID,
			})
			if err != nil {
				resp.Errors = append(resp.Errors, BatchResult{ID: item.ID, Status: http.StatusInternalServerError})
				continue
			}
			resp.Successes = append(resp.Successes, BatchResult{ID: item.ID, Status: http.StatusOK})

		default:
			resp.Errors = append(resp.Errors, BatchResult{ID: item.ID, Status: http.StatusBadRequest})
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// ListTraces handles GET /api/public/traces — SDK-compatible endpoint.
func (h *SDKHandler) ListTraces(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	limit, _ := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 32)
	offset, _ := strconv.ParseInt(r.URL.Query().Get("page"), 10, 32)
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

// GetTrace handles GET /api/public/traces/:traceId — SDK-compatible endpoint.
func (h *SDKHandler) GetTrace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "traceId")

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

// RegisterSDKRoutes registers SDK-compatible routes under /api/public.
func (h *SDKHandler) RegisterSDKRoutes(r chi.Router) {
	r.Post("/traces", h.CreateTrace)
	r.Get("/traces", h.ListTraces)
	r.Get("/traces/{traceId}", h.GetTrace)
	r.Post("/observations", h.CreateObservation)
	r.Post("/ingestion", h.BatchIngestion)
}
