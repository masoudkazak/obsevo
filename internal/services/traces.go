package services

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/langfuse-light/langfuse-light/internal/db"
)

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
}

// CreateTrace creates or updates a trace.
func (s *TraceService) CreateTrace(ctx context.Context, projectID string, req CreateTraceRequest) (db.Trace, error) {
	existing, err := s.queries.GetTraceByID(ctx, req.ID)
	if err == nil && existing.ID != "" {
		return s.updateTrace(ctx, req)
	}

	startTime := parseTime(req.StartTime)
	endTime := parseTime(req.EndTime)

	trace, err := s.queries.CreateTrace(ctx, db.CreateTraceParams{
		ID:         req.ID,
		ProjectID:  projectID,
		Name:       pgtype.Text{String: req.Name, Valid: req.Name != ""},
		Input:      req.Input,
		Output:     req.Output,
		Metadata:   req.Metadata,
		UserID:     pgtype.Text{String: req.UserID, Valid: req.UserID != ""},
		SessionID:  pgtype.Text{String: req.SessionID, Valid: req.SessionID != ""},
		Tags:       req.Tags,
		StartTime:  startTime,
		EndTime:    endTime,
		TotalCost:  pgtype.Float8{Float64: 0, Valid: req.TotalCost != nil},
		TokenUsage: req.TokenUsage,
	})
	if err != nil {
		return db.Trace{}, fmt.Errorf("creating trace: %w", err)
	}

	if req.TotalCost != nil {
		trace.TotalCost = pgtype.Float8{Float64: *req.TotalCost, Valid: true}
	}

	return trace, nil
}

func (s *TraceService) updateTrace(ctx context.Context, req CreateTraceRequest) (db.Trace, error) {
	trace, err := s.queries.UpdateTrace(ctx, db.UpdateTraceParams{
		ID:         req.ID,
		Name:       pgtype.Text{String: req.Name, Valid: req.Name != ""},
		Input:      req.Input,
		Output:     req.Output,
		Metadata:   req.Metadata,
		EndTime:    parseTime(req.EndTime),
		TotalCost:  pgtype.Float8{Valid: req.TotalCost != nil},
		TokenUsage: req.TokenUsage,
	})
	if err != nil {
		return db.Trace{}, fmt.Errorf("updating trace: %w", err)
	}
	return trace, nil
}

// GetTrace returns a trace by ID.
func (s *TraceService) GetTrace(ctx context.Context, id string) (db.Trace, error) {
	trace, err := s.queries.GetTraceByID(ctx, id)
	if err != nil {
		return db.Trace{}, fmt.Errorf("getting trace: %w", err)
	}
	return trace, nil
}

// GetTraceDetail returns a trace with its observations.
func (s *TraceService) GetTraceDetail(ctx context.Context, id string) (db.GetTraceWithProjectRow, []db.Observation, error) {
	trace, err := s.queries.GetTraceWithProject(ctx, id)
	if err != nil {
		return db.GetTraceWithProjectRow{}, nil, fmt.Errorf("getting trace: %w", err)
	}

	observations, err := s.queries.GetObservationsByTraceID(ctx, id)
	if err != nil {
		return db.GetTraceWithProjectRow{}, nil, fmt.Errorf("getting observations: %w", err)
	}

	return trace, observations, nil
}

// ListTracesRequest contains filters for listing traces.
type ListTracesRequest struct {
	ProjectID string
	Name      string
	Limit     int32
	Offset    int32
}

// ListTracesResponse is the paginated response for listing traces.
type ListTracesResponse struct {
	Traces []db.Trace `json:"traces"`
	Total  int64      `json:"total"`
}

// ListTraces lists traces for a project with optional filters.
func (s *TraceService) ListTraces(ctx context.Context, req ListTracesRequest) (ListTracesResponse, error) {
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 50
	}

	var traces []db.Trace
	var err error

	if req.Name != "" {
		traces, err = s.queries.GetTracesByProjectIDAndName(ctx, db.GetTracesByProjectIDAndNameParams{
			ProjectID: req.ProjectID,
			Name:      pgtype.Text{String: req.Name, Valid: true},
			Limit:     req.Limit,
			Offset:    req.Offset,
		})
	} else {
		traces, err = s.queries.GetTracesByProjectID(ctx, db.GetTracesByProjectIDParams{
			ProjectID: req.ProjectID,
			Limit:     req.Limit,
			Offset:    req.Offset,
		})
	}
	if err != nil {
		return ListTracesResponse{}, fmt.Errorf("listing traces: %w", err)
	}

	total, err := s.queries.CountTracesByProjectID(ctx, req.ProjectID)
	if err != nil {
		return ListTracesResponse{}, fmt.Errorf("counting traces: %w", err)
	}

	return ListTracesResponse{Traces: traces, Total: total}, nil
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
}

// CreateObservation creates an observation.
func (s *TraceService) CreateObservation(ctx context.Context, req CreateObservationRequest) (db.Observation, error) {
	if req.Type == "" {
		req.Type = "SPAN"
	}
	if req.Status == "" {
		req.Status = "DEFAULT"
	}

	obs, err := s.queries.CreateObservation(ctx, db.CreateObservationParams{
		TraceID:             req.TraceID,
		Type:                req.Type,
		Name:                pgtype.Text{String: req.Name, Valid: req.Name != ""},
		Input:               req.Input,
		Output:              req.Output,
		Metadata:            req.Metadata,
		Model:               pgtype.Text{String: req.Model, Valid: req.Model != ""},
		ModelParameters:     req.ModelParameters,
		StartTime:           parseTime(req.StartTime),
		EndTime:             parseTime(req.EndTime),
		TokenUsage:          req.TokenUsage,
		Cost:                pgtype.Float8{Valid: req.Cost != nil},
		Status:              req.Status,
		ParentObservationID: pgtype.Text{String: req.ParentObservationID, Valid: req.ParentObservationID != ""},
	})
	if err != nil {
		return db.Observation{}, fmt.Errorf("creating observation: %w", err)
	}

	if req.Cost != nil {
		obs.Cost = pgtype.Float8{Float64: *req.Cost, Valid: true}
	}

	return obs, nil
}

// GetObservation returns an observation by ID.
func (s *TraceService) GetObservation(ctx context.Context, id string) (db.Observation, error) {
	obs, err := s.queries.GetObservationByID(ctx, id)
	if err != nil {
		return db.Observation{}, fmt.Errorf("getting observation: %w", err)
	}
	return obs, nil
}

// GetObservationsByTraceID returns all observations for a trace.
func (s *TraceService) GetObservationsByTraceID(ctx context.Context, traceID string) ([]db.Observation, error) {
	obs, err := s.queries.GetObservationsByTraceID(ctx, traceID)
	if err != nil {
		return nil, fmt.Errorf("getting observations: %w", err)
	}
	return obs, nil
}

func parseTime(s string) pgtype.Timestamptz {
	if s == "" {
		return pgtype.Timestamptz{}
	}
	t := pgtype.Timestamptz{}
	_ = t.Scan(s)
	return t
}
