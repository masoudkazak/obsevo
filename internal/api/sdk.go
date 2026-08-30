package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/obsevo/obsevo/internal/auth"
	"github.com/obsevo/obsevo/internal/queue"
	"github.com/obsevo/obsevo/internal/services"
)

// maxBatchItems bounds a single ingestion batch. The Langfuse SDKs flush in
// batches well under this; the cap exists so one request cannot pin the process.
const maxBatchItems = 1000

// SDKHandler handles the Langfuse-compatible public API under /api/public.
type SDKHandler struct {
	traceService   *services.TraceService
	evalService    *services.EvaluationService
	promptService  *services.PromptService
	datasetService *services.DatasetService
	queue          *queue.Queue
}

// NewSDKHandler creates a new SDK handler.
func NewSDKHandler(
	traceService *services.TraceService,
	evalService *services.EvaluationService,
	promptService *services.PromptService,
	datasetService *services.DatasetService,
	q *queue.Queue,
) *SDKHandler {
	return &SDKHandler{
		traceService:   traceService,
		evalService:    evalService,
		promptService:  promptService,
		datasetService: datasetService,
		queue:          q,
	}
}

// ---------------------------------------------------------------------------
// Langfuse event bodies
// ---------------------------------------------------------------------------

// CreateTraceRequest represents the Langfuse SDK trace body (camelCase).
type CreateTraceRequest struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Input       json.RawMessage `json:"input"`
	Output      json.RawMessage `json:"output"`
	Metadata    json.RawMessage `json:"metadata"`
	UserID      string          `json:"userId"`
	SessionID   string          `json:"sessionId"`
	Tags        []string        `json:"tags"`
	StartTime   string          `json:"startTime"`
	EndTime     string          `json:"endTime"`
	Timestamp   string          `json:"timestamp"`
	Release     string          `json:"release"`
	Version     string          `json:"version"`
	Public      bool            `json:"public"`
	Environment string          `json:"environment"`
}

// toService converts a Langfuse trace body to the internal request shape.
func (r CreateTraceRequest) toService() services.CreateTraceRequest {
	startTime := r.StartTime
	if startTime == "" {
		// Langfuse names the trace's own clock field `timestamp`.
		startTime = r.Timestamp
	}
	return services.CreateTraceRequest{
		ID:          r.ID,
		Name:        r.Name,
		Input:       r.Input,
		Output:      r.Output,
		Metadata:    r.Metadata,
		UserID:      r.UserID,
		SessionID:   r.SessionID,
		Tags:        r.Tags,
		StartTime:   startTime,
		EndTime:     r.EndTime,
		Release:     r.Release,
		Version:     r.Version,
		Public:      r.Public,
		Environment: r.Environment,
	}
}

// CreateObservationRequest represents the Langfuse SDK observation body.
// One struct covers spans, generations and events: the SDKs send the same
// envelope and vary only which fields are populated.
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
	CompletionStartTime string          `json:"completionStartTime"`
	Usage               json.RawMessage `json:"usage"`
	UsageDetails        json.RawMessage `json:"usageDetails"`
	CostDetails         json.RawMessage `json:"costDetails"`
	Cost                *float64        `json:"cost"`
	Level               string          `json:"level"`
	StatusMessage       string          `json:"statusMessage"`
	Status              string          `json:"status"`
	ParentObservationID string          `json:"parentObservationId"`
	PromptName          string          `json:"promptName"`
	PromptVersion       *int32          `json:"promptVersion"`
	Version             string          `json:"version"`
	Environment         string          `json:"environment"`
}

// toService converts a Langfuse observation body to the internal request shape.
// defaultType supplies the observation kind implied by the event name, since
// `generation-create` and `span-create` carry no explicit `type` field.
func (r CreateObservationRequest) toService(defaultType string) services.CreateObservationRequest {
	obsType := r.Type
	if obsType == "" {
		obsType = defaultType
	}

	// Prefer the richer usageDetails when both are present.
	rawUsage := r.UsageDetails
	if len(rawUsage) == 0 {
		rawUsage = r.Usage
	}
	normalizedUsage, _ := services.NormalizeUsage(rawUsage)

	cost := r.Cost
	if cost == nil {
		cost = usageCost(r.Usage, r.CostDetails)
	}

	return services.CreateObservationRequest{
		ID:                  r.ID,
		TraceID:             r.TraceID,
		Type:                obsType,
		Name:                r.Name,
		Input:               r.Input,
		Output:              r.Output,
		Metadata:            r.Metadata,
		Model:               r.Model,
		ModelParameters:     r.ModelParameters,
		StartTime:           r.StartTime,
		EndTime:             r.EndTime,
		CompletionStartTime: r.CompletionStartTime,
		TokenUsage:          normalizedUsage,
		UsageDetails:        rawUsage,
		CostDetails:         r.CostDetails,
		Cost:                cost,
		Status:              r.Status,
		Level:               r.Level,
		StatusMessage:       r.StatusMessage,
		ParentObservationID: r.ParentObservationID,
		PromptName:          r.PromptName,
		PromptVersion:       r.PromptVersion,
		Version:             r.Version,
		Environment:         r.Environment,
	}
}

// usageCost extracts a client-reported cost from the Langfuse usage or
// costDetails payloads, which carry it under several historical names.
func usageCost(usage, costDetails json.RawMessage) *float64 {
	for _, raw := range []json.RawMessage{costDetails, usage} {
		if len(raw) == 0 {
			continue
		}
		var fields map[string]interface{}
		if err := json.Unmarshal(raw, &fields); err != nil {
			continue
		}
		for _, key := range []string{"totalCost", "total_cost", "total"} {
			if v, ok := fields[key].(float64); ok && v > 0 {
				cost := v
				return &cost
			}
		}
	}
	return nil
}

// CreateScoreRequest represents the Langfuse SDK score body. Langfuse permits
// `value` to be a number or a string depending on the score's data type, so it
// is decoded permissively.
type CreateScoreRequest struct {
	ID            string          `json:"id"`
	TraceID       string          `json:"traceId"`
	ObservationID string          `json:"observationId"`
	SessionID     string          `json:"sessionId"`
	DatasetRunID  string          `json:"datasetRunId"`
	Name          string          `json:"name"`
	Value         json.RawMessage `json:"value"`
	// StringValue is not part of the SDK write shape, but the read shape emits
	// it, so accepting it here lets a categorical score survive a round trip
	// through export and re-import.
	StringValue string          `json:"stringValue"`
	Comment     string          `json:"comment"`
	Source      string          `json:"source"`
	DataType    string          `json:"dataType"`
	ConfigID    string          `json:"configId"`
	Metadata    json.RawMessage `json:"metadata"`
}

// toService converts a Langfuse score body to the internal request shape.
func (r CreateScoreRequest) toService() services.CreateScoreRequest {
	req := services.CreateScoreRequest{
		ID:            r.ID,
		TraceID:       r.TraceID,
		ObservationID: r.ObservationID,
		SessionID:     r.SessionID,
		DatasetRunID:  r.DatasetRunID,
		Name:          r.Name,
		Comment:       r.Comment,
		// Langfuse labels SDK-submitted scores as API when the client omits a source.
		Source:   firstNonEmptyString(strings.ToUpper(r.Source), "API"),
		DataType: r.DataType,
		ConfigID: r.ConfigID,
		Metadata: r.Metadata,
	}

	req.StringValue = r.StringValue
	if len(r.Value) > 0 {
		var num float64
		if err := json.Unmarshal(r.Value, &num); err == nil {
			req.Value = &num
		} else {
			// Langfuse sends a categorical score's label in `value`.
			var str string
			if err := json.Unmarshal(r.Value, &str); err == nil {
				req.StringValue = str
			}
		}
	}

	return req
}

// ---------------------------------------------------------------------------
// Batch ingestion
// ---------------------------------------------------------------------------

// BatchIngestionBody is the body for SDK batch ingestion.
type BatchIngestionBody struct {
	Batch    []BatchItem     `json:"batch"`
	Metadata json.RawMessage `json:"metadata"`
}

// BatchItem represents a single event in a batch ingestion request.
type BatchItem struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Body      json.RawMessage `json:"body"`
}

// BatchIngestionResponse mirrors the Langfuse ingestion response envelope.
type BatchIngestionResponse struct {
	Successes []BatchResult `json:"successes"`
	Errors    []BatchError  `json:"errors"`
}

// BatchResult identifies an accepted event.
type BatchResult struct {
	ID     string `json:"id"`
	Status int    `json:"status"`
}

// BatchError identifies a rejected event and why.
type BatchError struct {
	ID      string `json:"id"`
	Status  int    `json:"status"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

// BatchIngestion handles POST /api/public/ingestion.
//
// Events are validated synchronously and then queued, so the SDK's flush
// returns without waiting on database writes. The reply is 207 Multi-Status
// with per-event results, matching Langfuse.
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

	if len(body.Batch) > maxBatchItems {
		writeError(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("batch exceeds %d events", maxBatchItems))
		return
	}

	resp := BatchIngestionResponse{
		Successes: []BatchResult{},
		Errors:    []BatchError{},
	}
	items := make([]queue.IngestionItem, 0, len(body.Batch))

	for _, event := range body.Batch {
		item, err := h.convertEvent(projectID, event)
		if err != nil {
			resp.Errors = append(resp.Errors, BatchError{
				ID:      event.ID,
				Status:  http.StatusBadRequest,
				Message: err.Error(),
			})
			continue
		}
		if item == nil {
			// Accepted and intentionally not persisted (for example sdk-log).
			resp.Successes = append(resp.Successes, BatchResult{ID: event.ID, Status: http.StatusOK})
			continue
		}
		items = append(items, *item)
		resp.Successes = append(resp.Successes, BatchResult{ID: event.ID, Status: http.StatusOK})
	}

	if err := h.queue.EnqueueBatch(r.Context(), items); err != nil {
		writeError(w, http.StatusServiceUnavailable, "ingestion queue unavailable: "+err.Error())
		return
	}

	writeJSON(w, http.StatusMultiStatus, resp)
}

// convertEvent maps one Langfuse batch event onto a queue item. A nil item with
// a nil error means the event is accepted but deliberately not stored.
func (h *SDKHandler) convertEvent(projectID string, event BatchItem) (*queue.IngestionItem, error) {
	// Langfuse observation kinds and the observation type each implies.
	observationTypes := map[string]string{
		"span-create":        "SPAN",
		"span-update":        "SPAN",
		"generation-create":  "GENERATION",
		"generation-update":  "GENERATION",
		"event-create":       "EVENT",
		"observation-create": "SPAN",
		"observation-update": "SPAN",
	}

	switch event.Type {
	case "trace-create", "trace-update":
		var body CreateTraceRequest
		if err := json.Unmarshal(event.Body, &body); err != nil {
			return nil, fmt.Errorf("invalid trace body: %w", err)
		}
		if body.ID == "" {
			body.ID = event.ID
		}
		if body.ID == "" {
			return nil, fmt.Errorf("trace id is required")
		}
		req := body.toService()
		if req.StartTime == "" {
			req.StartTime = event.Timestamp
		}
		return newQueueItem(event.ID, "trace", projectID, req)

	case "score-create":
		var body CreateScoreRequest
		if err := json.Unmarshal(event.Body, &body); err != nil {
			return nil, fmt.Errorf("invalid score body: %w", err)
		}
		if body.Name == "" {
			return nil, fmt.Errorf("score name is required")
		}
		if body.TraceID == "" && body.ObservationID == "" && body.SessionID == "" && body.DatasetRunID == "" {
			return nil, fmt.Errorf("score requires one of traceId, observationId, sessionId or datasetRunId")
		}
		return newQueueItem(event.ID, "score", projectID, body.toService())

	case "sdk-log":
		// Client-side diagnostics. Accepted so SDK flushes succeed, but there
		// is no value in persisting them in a lightweight deployment.
		return nil, nil

	default:
		defaultType, ok := observationTypes[event.Type]
		if !ok {
			return nil, fmt.Errorf("unsupported event type %q", event.Type)
		}

		var body CreateObservationRequest
		if err := json.Unmarshal(event.Body, &body); err != nil {
			return nil, fmt.Errorf("invalid observation body: %w", err)
		}
		if body.TraceID == "" {
			return nil, fmt.Errorf("traceId is required")
		}
		req := body.toService(defaultType)
		if req.StartTime == "" {
			req.StartTime = event.Timestamp
		}
		return newQueueItem(event.ID, "observation", projectID, req)
	}
}

// newQueueItem serialises a converted event for the ingestion queue.
func newQueueItem(id, kind, projectID string, payload interface{}) (*queue.IngestionItem, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encoding %s event: %w", kind, err)
	}
	return &queue.IngestionItem{
		ID:        id,
		Type:      kind,
		Payload:   encoded,
		ProjectID: projectID,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// ---------------------------------------------------------------------------
// Traces
// ---------------------------------------------------------------------------

// CreateTraceResponse represents the response for trace creation.
type CreateTraceResponse struct {
	ID string `json:"id"`
}

// CreateTrace handles POST /api/public/traces.
// Unlike batch ingestion this writes synchronously, because the caller expects
// the trace to be readable as soon as the call returns.
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

	trace, err := h.traceService.CreateTrace(r.Context(), projectID, req.toService())
	if err != nil {
		writeServiceError(w, err, "trace not found")
		return
	}

	writeJSON(w, http.StatusCreated, CreateTraceResponse{ID: trace.ID})
}

// ListTraces handles GET /api/public/traces.
func (h *SDKHandler) ListTraces(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	q := r.URL.Query()
	limit := queryInt32(r, "limit", 50)
	resp, err := h.traceService.ListTraces(r.Context(), services.ListTracesRequest{
		ProjectID:   projectID,
		Name:        q.Get("name"),
		UserID:      q.Get("userId"),
		SessionID:   q.Get("sessionId"),
		Release:     q.Get("release"),
		Version:     q.Get("version"),
		Environment: q.Get("environment"),
		Tags:        queryCSV(r, "tags"),
		FromTime:    q.Get("fromTimestamp"),
		ToTime:      q.Get("toTimestamp"),
		Limit:       limit,
		Offset:      pageOffset(r, limit),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list traces")
		return
	}

	writeJSON(w, http.StatusOK, langfusePage(toPublicTraces(resp.Traces), resp.Total, resp.Limit, resp.Offset))
}

// GetTrace handles GET /api/public/traces/{traceId}.
func (h *SDKHandler) GetTrace(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	traceID := chi.URLParam(r, "traceId")

	trace, observations, err := h.traceService.GetTraceDetail(r.Context(), projectID, traceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "trace not found")
		return
	}

	// Langfuse returns the trace object itself with nested observations and
	// scores, not a wrapper around them.
	payload := toPublicTrace(trace)
	payload.Observations = toPublicObservations(observations)

	scores, err := h.evalService.ListScores(r.Context(), services.ListScoresRequest{
		ProjectID: projectID,
		TraceID:   traceID,
		Limit:     500,
	})
	if err == nil {
		payload.Scores = toPublicScores(scores.Scores)
	}

	writeJSON(w, http.StatusOK, payload)
}

// DeleteTrace handles DELETE /api/public/traces/{traceId}.
func (h *SDKHandler) DeleteTrace(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	if err := h.traceService.DeleteTrace(r.Context(), projectID, chi.URLParam(r, "traceId")); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete trace")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ---------------------------------------------------------------------------
// Observations
// ---------------------------------------------------------------------------

// CreateObservationResponse represents the response for observation creation.
type CreateObservationResponse struct {
	ID string `json:"id"`
}

// CreateObservation handles POST /api/public/observations.
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

	obs, err := h.traceService.CreateObservation(r.Context(), projectID, req.toService("SPAN"))
	if err != nil {
		writeServiceError(w, err, "observation not found")
		return
	}

	writeJSON(w, http.StatusCreated, CreateObservationResponse{ID: obs.ID})
}

// ListObservations handles GET /api/public/observations.
func (h *SDKHandler) ListObservations(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	q := r.URL.Query()
	limit := queryInt32(r, "limit", 50)
	resp, err := h.traceService.ListObservations(r.Context(), services.ListObservationsRequest{
		ProjectID: projectID,
		TraceID:   q.Get("traceId"),
		Type:      q.Get("type"),
		Name:      q.Get("name"),
		Model:     q.Get("model"),
		Level:     q.Get("level"),
		FromTime:  q.Get("fromStartTime"),
		ToTime:    q.Get("toStartTime"),
		Limit:     limit,
		Offset:    pageOffset(r, limit),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list observations")
		return
	}

	writeJSON(w, http.StatusOK, langfusePage(toPublicObservations(resp.Observations), resp.Total, resp.Limit, resp.Offset))
}

// GetObservation handles GET /api/public/observations/{observationId}.
func (h *SDKHandler) GetObservation(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	obs, err := h.traceService.GetObservation(r.Context(), projectID, chi.URLParam(r, "observationId"))
	if err != nil {
		writeError(w, http.StatusNotFound, "observation not found")
		return
	}

	writeJSON(w, http.StatusOK, toPublicObservation(obs))
}

// ---------------------------------------------------------------------------
// Scores
// ---------------------------------------------------------------------------

// CreateScore handles POST /api/public/scores.
func (h *SDKHandler) CreateScore(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	var req CreateScoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	score, err := h.evalService.CreateScore(r.Context(), projectID, req.toService())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, toPublicScore(score))
}

// ListScores handles GET /api/public/scores.
func (h *SDKHandler) ListScores(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	q := r.URL.Query()
	limit := queryInt32(r, "limit", 50)
	resp, err := h.evalService.ListScores(r.Context(), services.ListScoresRequest{
		ProjectID:     projectID,
		Name:          q.Get("name"),
		Source:        strings.ToUpper(q.Get("source")),
		TraceID:       q.Get("traceId"),
		ObservationID: q.Get("observationId"),
		DataType:      q.Get("dataType"),
		Limit:         limit,
		Offset:        pageOffset(r, limit),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list scores")
		return
	}

	writeJSON(w, http.StatusOK, langfusePage(toPublicScores(resp.Scores), resp.Total, resp.Limit, resp.Offset))
}

// GetScore handles GET /api/public/scores/{scoreId}.
func (h *SDKHandler) GetScore(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	score, err := h.evalService.GetScore(r.Context(), projectID, chi.URLParam(r, "scoreId"))
	if err != nil {
		writeError(w, http.StatusNotFound, "score not found")
		return
	}

	writeJSON(w, http.StatusOK, toPublicScore(score))
}

// DeleteScore handles DELETE /api/public/scores/{scoreId}.
func (h *SDKHandler) DeleteScore(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	if err := h.evalService.DeleteScore(r.Context(), projectID, chi.URLParam(r, "scoreId")); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete score")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ---------------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------------

// ListSessions handles GET /api/public/sessions.
func (h *SDKHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	limit := queryInt32(r, "limit", 50)
	resp, err := h.traceService.ListSessions(r.Context(), projectID, limit, pageOffset(r, limit))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}

	writeJSON(w, http.StatusOK, langfusePage(toPublicSessions(resp.Sessions), resp.Total, resp.Limit, resp.Offset))
}

// GetSession handles GET /api/public/sessions/{sessionId}.
func (h *SDKHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusUnauthorized, "valid API key required")
		return
	}

	session, traces, err := h.traceService.GetSessionDetail(r.Context(), projectID, chi.URLParam(r, "sessionId"))
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	writeJSON(w, http.StatusOK, PublicSession{
		ID:          session.ID,
		ProjectID:   session.ProjectID,
		CreatedAt:   formatTime(session.CreatedAt),
		Environment: session.Environment,
		Bookmarked:  session.Bookmarked,
		Public:      session.Public,
		TraceCount:  int64(len(traces)),
		Traces:      toPublicTraces(traces),
	})
}

// Health handles GET /api/public/health.
func (h *SDKHandler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "OK",
		"version": "obsevo",
	})
}

// ---------------------------------------------------------------------------
// Shared pagination helpers
// ---------------------------------------------------------------------------

// pageOffset converts Langfuse's 1-based `page` parameter into an offset,
// falling back to an explicit `offset` for callers using this project's own
// pagination style.
func pageOffset(r *http.Request, limit int32) int32 {
	if r.URL.Query().Get("page") != "" {
		page := queryInt32(r, "page", 1)
		if page < 1 {
			page = 1
		}
		return (page - 1) * limit
	}
	return queryInt32(r, "offset", 0)
}

// firstNonEmptyString returns the first non-empty value.
func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// langfusePage wraps a result set in Langfuse's `{data, meta}` envelope.
func langfusePage(data interface{}, total int64, limit, offset int32) map[string]interface{} {
	page := int32(1)
	totalPages := int32(1)
	if limit > 0 {
		page = offset/limit + 1
		totalPages = int32((total + int64(limit) - 1) / int64(limit))
		if totalPages < 1 {
			totalPages = 1
		}
	}

	return map[string]interface{}{
		"data": data,
		"meta": map[string]interface{}{
			"page":       page,
			"limit":      limit,
			"totalItems": total,
			"totalPages": totalPages,
		},
	}
}

// RegisterSDKRoutes registers the Langfuse-compatible routes under /api/public.
func (h *SDKHandler) RegisterSDKRoutes(r chi.Router) {
	r.Get("/health", h.Health)

	r.Post("/ingestion", h.BatchIngestion)

	r.Post("/traces", h.CreateTrace)
	r.Get("/traces", h.ListTraces)
	r.Get("/traces/{traceId}", h.GetTrace)
	r.Delete("/traces/{traceId}", h.DeleteTrace)

	r.Post("/observations", h.CreateObservation)
	r.Get("/observations", h.ListObservations)
	r.Get("/observations/{observationId}", h.GetObservation)

	r.Post("/scores", h.CreateScore)
	r.Get("/scores", h.ListScores)
	r.Get("/scores/{scoreId}", h.GetScore)
	r.Delete("/scores/{scoreId}", h.DeleteScore)

	r.Get("/sessions", h.ListSessions)
	r.Get("/sessions/{sessionId}", h.GetSession)

	h.RegisterResourceRoutes(r)
}
