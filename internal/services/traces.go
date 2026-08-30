package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/langfuse-light/langfuse-light/internal/db"
)

// ErrCrossProject is returned when a request references an entity that exists
// but belongs to a different project than the caller is authenticated for.
var ErrCrossProject = errors.New("entity belongs to a different project")

// defaultEnvironment matches Langfuse's default tracing environment name.
const defaultEnvironment = "default"

// TraceService handles trace and observation business logic.
type TraceService struct {
	queries *db.Queries
}

// NewTraceService creates a new trace service.
func NewTraceService(queries *db.Queries) *TraceService {
	return &TraceService{queries: queries}
}

// CreateTraceRequest is the request body for creating a trace.
type CreateTraceRequest struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Input      json.RawMessage `json:"input"`
	Output     json.RawMessage `json:"output"`
	Metadata   json.RawMessage `json:"metadata"`
	UserID     string          `json:"user_id"`
	SessionID  string          `json:"session_id"`
	Tags       []string        `json:"tags"`
	StartTime  string          `json:"start_time"`
	EndTime    string          `json:"end_time"`
	TotalCost  *float64        `json:"total_cost"`
	TokenUsage json.RawMessage `json:"token_usage"`

	// Langfuse-compatible fields.
	Release     string `json:"release"`
	Version     string `json:"version"`
	Public      bool   `json:"public"`
	Bookmarked  bool   `json:"bookmarked"`
	Environment string `json:"environment"`
}

// CreateTrace creates a trace, or merges the request into an existing one.
// Ingestion is idempotent: SDKs retry and reorder events freely.
func (s *TraceService) CreateTrace(ctx context.Context, projectID string, req CreateTraceRequest) (db.Trace, error) {
	if req.ID == "" {
		return db.Trace{}, fmt.Errorf("trace id is required")
	}

	if err := s.assertTraceOwnership(ctx, projectID, req.ID); err != nil {
		return db.Trace{}, err
	}

	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}

	trace, err := s.queries.UpsertTrace(ctx, db.UpsertTraceParams{
		ID:          req.ID,
		ProjectID:   projectID,
		Name:        req.Name,
		Input:       req.Input,
		Output:      req.Output,
		Metadata:    req.Metadata,
		UserID:      req.UserID,
		SessionID:   req.SessionID,
		Tags:        tags,
		StartTime:   parseTimeOrNow(req.StartTime),
		EndTime:     parseTime(req.EndTime),
		TotalCost:   nullableFloat(req.TotalCost),
		TokenUsage:  req.TokenUsage,
		Release:     req.Release,
		Version:     req.Version,
		Public:      req.Public,
		Bookmarked:  req.Bookmarked,
		Environment: environmentOrDefault(req.Environment),
	})
	if err != nil {
		return db.Trace{}, fmt.Errorf("upserting trace: %w", err)
	}

	if req.SessionID != "" {
		if err := s.queries.UpsertSession(ctx, db.UpsertSessionParams{
			ID:          req.SessionID,
			ProjectID:   projectID,
			Environment: environmentOrDefault(req.Environment),
		}); err != nil {
			return db.Trace{}, fmt.Errorf("upserting session: %w", err)
		}
	}

	return trace, nil
}

// assertTraceOwnership fails if the trace id is already used by another project.
func (s *TraceService) assertTraceOwnership(ctx context.Context, projectID, traceID string) error {
	existing, err := s.queries.GetTraceByID(ctx, traceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("checking trace ownership: %w", err)
	}
	if existing.ProjectID != projectID {
		return ErrCrossProject
	}
	return nil
}

// ensureTrace creates a placeholder trace when an observation arrives before
// the trace that owns it — a normal SDK ordering, since spans are flushed as
// they close while the trace is only finalised at the end.
func (s *TraceService) ensureTrace(ctx context.Context, projectID, traceID string) error {
	existing, err := s.queries.GetTraceByID(ctx, traceID)
	if err == nil {
		if existing.ProjectID != projectID {
			return ErrCrossProject
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("looking up trace: %w", err)
	}

	_, err = s.queries.UpsertTrace(ctx, db.UpsertTraceParams{
		ID:          traceID,
		ProjectID:   projectID,
		Tags:        []string{},
		StartTime:   pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		Environment: defaultEnvironment,
	})
	if err != nil {
		return fmt.Errorf("creating placeholder trace: %w", err)
	}
	return nil
}

// GetTrace returns a trace by ID, scoped to a project.
func (s *TraceService) GetTrace(ctx context.Context, projectID, id string) (db.Trace, error) {
	trace, err := s.queries.GetTraceByIDAndProject(ctx, db.GetTraceByIDAndProjectParams{
		ID:        id,
		ProjectID: projectID,
	})
	if err != nil {
		return db.Trace{}, fmt.Errorf("getting trace: %w", err)
	}
	return trace, nil
}

// GetTraceDetail returns a trace with its observations, scoped to a project.
func (s *TraceService) GetTraceDetail(ctx context.Context, projectID, id string) (db.Trace, []db.Observation, error) {
	trace, err := s.queries.GetTraceByIDAndProject(ctx, db.GetTraceByIDAndProjectParams{
		ID:        id,
		ProjectID: projectID,
	})
	if err != nil {
		return db.Trace{}, nil, fmt.Errorf("getting trace: %w", err)
	}

	observations, err := s.queries.GetObservationsByTraceIDAndProject(ctx, db.GetObservationsByTraceIDAndProjectParams{
		TraceID:   id,
		ProjectID: projectID,
	})
	if err != nil {
		return db.Trace{}, nil, fmt.Errorf("getting observations: %w", err)
	}

	return trace, observations, nil
}

// DeleteTrace removes a trace and, by cascade, its observations and scores.
func (s *TraceService) DeleteTrace(ctx context.Context, projectID, id string) error {
	if err := s.queries.DeleteTrace(ctx, db.DeleteTraceParams{ID: id, ProjectID: projectID}); err != nil {
		return fmt.Errorf("deleting trace: %w", err)
	}
	return nil
}

// ListTracesRequest contains filters for listing traces.
type ListTracesRequest struct {
	ProjectID   string
	Name        string
	UserID      string
	SessionID   string
	Release     string
	Version     string
	Environment string
	Tags        []string
	FromTime    string
	ToTime      string
	Limit       int32
	Offset      int32
}

// ListTracesResponse is the paginated response for listing traces.
type ListTracesResponse struct {
	Traces []db.Trace `json:"traces"`
	Total  int64      `json:"total"`
	Limit  int32      `json:"limit"`
	Offset int32      `json:"offset"`
}

// ListTraces lists traces for a project with optional filters.
func (s *TraceService) ListTraces(ctx context.Context, req ListTracesRequest) (ListTracesResponse, error) {
	limit, offset := normalizePagination(req.Limit, req.Offset)

	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}
	from := parseTime(req.FromTime)
	to := parseTime(req.ToTime)

	traces, err := s.queries.ListTracesFiltered(ctx, db.ListTracesFilteredParams{
		ProjectID:   req.ProjectID,
		Name:        req.Name,
		UserID:      req.UserID,
		SessionID:   req.SessionID,
		Release:     req.Release,
		Version:     req.Version,
		Environment: req.Environment,
		Tags:        tags,
		FromTime:    from,
		ToTime:      to,
		Limit:       limit,
		Offset:      offset,
	})
	if err != nil {
		return ListTracesResponse{}, fmt.Errorf("listing traces: %w", err)
	}

	total, err := s.queries.CountTracesFiltered(ctx, db.CountTracesFilteredParams{
		ProjectID:   req.ProjectID,
		Name:        req.Name,
		UserID:      req.UserID,
		SessionID:   req.SessionID,
		Release:     req.Release,
		Version:     req.Version,
		Environment: req.Environment,
		Tags:        tags,
		FromTime:    from,
		ToTime:      to,
	})
	if err != nil {
		return ListTracesResponse{}, fmt.Errorf("counting traces: %w", err)
	}

	return ListTracesResponse{Traces: traces, Total: total, Limit: limit, Offset: offset}, nil
}

// CreateObservationRequest is the request body for creating an observation.
type CreateObservationRequest struct {
	ID                  string          `json:"id"`
	TraceID             string          `json:"trace_id"`
	Type                string          `json:"type"`
	Name                string          `json:"name"`
	Input               json.RawMessage `json:"input"`
	Output              json.RawMessage `json:"output"`
	Metadata            json.RawMessage `json:"metadata"`
	Model               string          `json:"model"`
	ModelParameters     json.RawMessage `json:"model_parameters"`
	StartTime           string          `json:"start_time"`
	EndTime             string          `json:"end_time"`
	TokenUsage          json.RawMessage `json:"token_usage"`
	Cost                *float64        `json:"cost"`
	Status              string          `json:"status"`
	ParentObservationID string          `json:"parent_observation_id"`

	// Langfuse-compatible fields.
	Level               string          `json:"level"`
	StatusMessage       string          `json:"status_message"`
	CompletionStartTime string          `json:"completion_start_time"`
	UsageDetails        json.RawMessage `json:"usage_details"`
	CostDetails         json.RawMessage `json:"cost_details"`
	PromptID            string          `json:"prompt_id"`
	PromptName          string          `json:"prompt_name"`
	PromptVersion       *int32          `json:"prompt_version"`
	Version             string          `json:"version"`
	Environment         string          `json:"environment"`
}

// CreateObservation creates or updates an observation within a project.
func (s *TraceService) CreateObservation(ctx context.Context, projectID string, req CreateObservationRequest) (db.Observation, error) {
	if req.TraceID == "" {
		return db.Observation{}, fmt.Errorf("trace_id is required")
	}
	if err := s.ensureTrace(ctx, projectID, req.TraceID); err != nil {
		return db.Observation{}, err
	}

	obsType := normalizeObservationType(req.Type)
	level, status := normalizeLevelAndStatus(req.Level, req.Status)

	obs, err := s.queries.UpsertObservation(ctx, db.UpsertObservationParams{
		ID:                  req.ID,
		TraceID:             req.TraceID,
		ProjectID:           projectID,
		Type:                obsType,
		Name:                req.Name,
		Input:               req.Input,
		Output:              req.Output,
		Metadata:            req.Metadata,
		Model:               req.Model,
		ModelParameters:     req.ModelParameters,
		StartTime:           parseTimeOrNow(req.StartTime),
		EndTime:             parseTime(req.EndTime),
		CompletionStartTime: parseTime(req.CompletionStartTime),
		TokenUsage:          req.TokenUsage,
		UsageDetails:        req.UsageDetails,
		CostDetails:         req.CostDetails,
		Cost:                nullableFloat(req.Cost),
		Status:              status,
		Level:               level,
		StatusMessage:       req.StatusMessage,
		ParentObservationID: req.ParentObservationID,
		PromptID:            req.PromptID,
		PromptName:          req.PromptName,
		PromptVersion:       nullableInt32(req.PromptVersion),
		Version:             req.Version,
		Environment:         environmentOrDefault(req.Environment),
	})
	if err != nil {
		return db.Observation{}, fmt.Errorf("upserting observation: %w", err)
	}

	return obs, nil
}

// RecalculateTraceAggregates rolls observation cost and tokens up onto the trace.
func (s *TraceService) RecalculateTraceAggregates(ctx context.Context, projectID, traceID string) error {
	err := s.queries.RecalculateTraceAggregates(ctx, db.RecalculateTraceAggregatesParams{
		TraceID:   traceID,
		ProjectID: projectID,
	})
	if err != nil {
		return fmt.Errorf("recalculating trace aggregates: %w", err)
	}
	return nil
}

// GetObservation returns an observation by ID, scoped to a project.
func (s *TraceService) GetObservation(ctx context.Context, projectID, id string) (db.Observation, error) {
	obs, err := s.queries.GetObservationByIDAndProject(ctx, db.GetObservationByIDAndProjectParams{
		ID:        id,
		ProjectID: projectID,
	})
	if err != nil {
		return db.Observation{}, fmt.Errorf("getting observation: %w", err)
	}
	return obs, nil
}

// GetObservationsByTraceID returns all observations for a trace in a project.
func (s *TraceService) GetObservationsByTraceID(ctx context.Context, projectID, traceID string) ([]db.Observation, error) {
	obs, err := s.queries.GetObservationsByTraceIDAndProject(ctx, db.GetObservationsByTraceIDAndProjectParams{
		TraceID:   traceID,
		ProjectID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("getting observations: %w", err)
	}
	return obs, nil
}

// ListObservationsRequest contains filters for listing observations.
type ListObservationsRequest struct {
	ProjectID string
	TraceID   string
	Type      string
	Name      string
	Model     string
	Level     string
	FromTime  string
	ToTime    string
	Limit     int32
	Offset    int32
}

// ListObservationsResponse is the paginated response for listing observations.
type ListObservationsResponse struct {
	Observations []db.Observation `json:"observations"`
	Total        int64            `json:"total"`
	Limit        int32            `json:"limit"`
	Offset       int32            `json:"offset"`
}

// ListObservations lists observations for a project with optional filters.
func (s *TraceService) ListObservations(ctx context.Context, req ListObservationsRequest) (ListObservationsResponse, error) {
	limit, offset := normalizePagination(req.Limit, req.Offset)

	observations, err := s.queries.ListObservationsFiltered(ctx, db.ListObservationsFilteredParams{
		ProjectID: req.ProjectID,
		TraceID:   req.TraceID,
		Type:      strings.ToUpper(req.Type),
		Name:      req.Name,
		Model:     req.Model,
		Level:     strings.ToUpper(req.Level),
		FromTime:  parseTime(req.FromTime),
		ToTime:    parseTime(req.ToTime),
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return ListObservationsResponse{}, fmt.Errorf("listing observations: %w", err)
	}

	total, err := s.queries.CountObservationsFiltered(ctx, db.CountObservationsFilteredParams{
		ProjectID: req.ProjectID,
		TraceID:   req.TraceID,
		Type:      strings.ToUpper(req.Type),
		Name:      req.Name,
		Model:     req.Model,
		Level:     strings.ToUpper(req.Level),
	})
	if err != nil {
		return ListObservationsResponse{}, fmt.Errorf("counting observations: %w", err)
	}

	return ListObservationsResponse{Observations: observations, Total: total, Limit: limit, Offset: offset}, nil
}

// ListSessionsResponse is the paginated response for listing sessions.
type ListSessionsResponse struct {
	Sessions []db.ListSessionsRow `json:"sessions"`
	Total    int64                `json:"total"`
	Limit    int32                `json:"limit"`
	Offset   int32                `json:"offset"`
}

// ListSessions lists sessions for a project with trace counts and cost totals.
func (s *TraceService) ListSessions(ctx context.Context, projectID string, limit, offset int32) (ListSessionsResponse, error) {
	limit, offset = normalizePagination(limit, offset)

	sessions, err := s.queries.ListSessions(ctx, db.ListSessionsParams{
		ProjectID: projectID,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return ListSessionsResponse{}, fmt.Errorf("listing sessions: %w", err)
	}

	total, err := s.queries.CountSessions(ctx, projectID)
	if err != nil {
		return ListSessionsResponse{}, fmt.Errorf("counting sessions: %w", err)
	}

	return ListSessionsResponse{Sessions: sessions, Total: total, Limit: limit, Offset: offset}, nil
}

// GetSessionDetail returns a session with the traces it groups.
func (s *TraceService) GetSessionDetail(ctx context.Context, projectID, id string) (db.Session, []db.Trace, error) {
	session, err := s.queries.GetSession(ctx, db.GetSessionParams{ID: id, ProjectID: projectID})
	if err != nil {
		return db.Session{}, nil, fmt.Errorf("getting session: %w", err)
	}

	traces, err := s.queries.GetTracesBySessionID(ctx, db.GetTracesBySessionIDParams{
		ProjectID: projectID,
		SessionID: pgtype.Text{String: id, Valid: true},
	})
	if err != nil {
		return db.Session{}, nil, fmt.Errorf("getting session traces: %w", err)
	}

	return session, traces, nil
}

// --- shared helpers -------------------------------------------------------

// timeLayouts are the timestamp formats accepted on ingestion, in match order.
// RFC3339 is what every Langfuse SDK emits; the remaining layouts cover the
// Postgres wire format and naive datetimes sent by hand-rolled clients.
var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05.999999999Z07:00",
	// Postgres renders a timestamptz offset as "+00", not "+00:00".
	"2006-01-02 15:04:05.999999999Z07",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02",
}

// ParseIngestionTime parses a timestamp sent by a client, reporting whether any
// accepted layout matched. Every SDK emits RFC 3339; the remaining layouts are
// there so a hand-rolled client is not silently rejected.
func ParseIngestionTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// parseTime converts an ingestion timestamp string to a Timestamptz.
// It returns an invalid (NULL) value when s is empty or unparseable.
func parseTime(s string) pgtype.Timestamptz {
	if parsed, ok := ParseIngestionTime(s); ok {
		return pgtype.Timestamptz{Time: parsed, Valid: true}
	}
	return pgtype.Timestamptz{}
}

// parseTimeOrNow behaves like parseTime but falls back to the current time,
// for the NOT NULL start_time columns where a client omitted the value.
func parseTimeOrNow(s string) pgtype.Timestamptz {
	if ts := parseTime(s); ts.Valid {
		return ts
	}
	return pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
}

// nullableFloat converts an optional float to a Float8, leaving it NULL when nil.
func nullableFloat(v *float64) pgtype.Float8 {
	if v == nil {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: *v, Valid: true}
}

// nullableInt32 converts an optional int32 to an Int4, leaving it NULL when nil.
func nullableInt32(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
}

// environmentOrDefault falls back to Langfuse's "default" tracing environment.
func environmentOrDefault(env string) string {
	if env == "" {
		return defaultEnvironment
	}
	return env
}

// normalizePagination clamps limit to a sane page size and offset to >= 0.
func normalizePagination(limit, offset int32) (int32, int32) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// normalizeObservationType maps SDK type names onto the stored enum.
func normalizeObservationType(t string) string {
	switch strings.ToUpper(t) {
	case "GENERATION":
		return "GENERATION"
	case "EVENT":
		return "EVENT"
	case "SPAN", "":
		return "SPAN"
	default:
		// Agent, tool, retriever and similar Langfuse subtypes are all spans in
		// storage; the specific kind stays in the observation name/metadata.
		return "SPAN"
	}
}

// normalizeLevelAndStatus keeps the Langfuse `level` field and the original
// `status` column consistent, whichever one the client supplied.
func normalizeLevelAndStatus(level, status string) (string, string) {
	level = strings.ToUpper(level)
	status = strings.ToUpper(status)

	switch level {
	case "DEBUG", "DEFAULT", "WARNING", "ERROR":
	default:
		level = ""
	}
	switch status {
	case "DEFAULT", "OK", "ERROR":
	default:
		status = ""
	}

	switch {
	case level == "" && status == "":
		return "DEFAULT", "DEFAULT"
	case level == "":
		if status == "ERROR" {
			return "ERROR", status
		}
		return "DEFAULT", status
	case status == "":
		if level == "ERROR" {
			return level, "ERROR"
		}
		return level, "DEFAULT"
	default:
		return level, status
	}
}
