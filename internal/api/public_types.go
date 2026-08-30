package api

import (
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/langfuse-light/langfuse-light/internal/db"
	"github.com/langfuse-light/langfuse-light/internal/services"
)

// The dashboard API returns stored rows as-is, in snake_case — that is its
// established contract and existing clients depend on it. The public API is a
// different contract: it is what the Langfuse SDKs and migration tooling read,
// and they expect Langfuse's camelCase field names. These types perform that
// translation, so compatibility is a property of the response shape rather than
// something each caller has to work around.

// PublicTrace is a trace in Langfuse's response shape.
type PublicTrace struct {
	ID          string          `json:"id"`
	Timestamp   string          `json:"timestamp"`
	Name        string          `json:"name,omitempty"`
	Input       json.RawMessage `json:"input,omitempty"`
	Output      json.RawMessage `json:"output,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	UserID      string          `json:"userId,omitempty"`
	SessionID   string          `json:"sessionId,omitempty"`
	Tags        []string        `json:"tags"`
	Release     string          `json:"release,omitempty"`
	Version     string          `json:"version,omitempty"`
	Public      bool            `json:"public"`
	Bookmarked  bool            `json:"bookmarked"`
	Environment string          `json:"environment"`
	ProjectID   string          `json:"projectId"`

	// Latency is in seconds, and is null until the trace has an end time.
	Latency   *float64 `json:"latency,omitempty"`
	TotalCost *float64 `json:"totalCost,omitempty"`
	CreatedAt string   `json:"createdAt"`
	UpdatedAt string   `json:"updatedAt"`

	// HTMLPath is where a human can open this trace, which is what the SDKs
	// print when a developer asks for a link to what they just traced.
	HTMLPath string `json:"htmlPath"`

	Observations []PublicObservation `json:"observations,omitempty"`
	Scores       []PublicScore       `json:"scores,omitempty"`
}

// toPublicTrace converts a stored trace.
func toPublicTrace(trace db.Trace) PublicTrace {
	out := PublicTrace{
		ID:          trace.ID,
		Timestamp:   formatTime(trace.StartTime),
		Name:        trace.Name.String,
		Input:       trace.Input,
		Output:      trace.Output,
		Metadata:    trace.Metadata,
		UserID:      trace.UserID.String,
		SessionID:   trace.SessionID.String,
		Tags:        trace.Tags,
		Release:     trace.Release.String,
		Version:     trace.Version.String,
		Public:      trace.Public,
		Bookmarked:  trace.Bookmarked,
		Environment: trace.Environment,
		ProjectID:   trace.ProjectID,
		CreatedAt:   formatTime(trace.CreatedAt),
		UpdatedAt:   formatTime(trace.UpdatedAt),
		HTMLPath:    "/traces/" + trace.ID,
	}

	if out.Tags == nil {
		out.Tags = []string{}
	}
	if trace.TotalCost.Valid {
		cost := trace.TotalCost.Float64
		out.TotalCost = &cost
	}
	if latency, ok := durationSeconds(trace.StartTime, trace.EndTime); ok {
		out.Latency = &latency
	}

	return out
}

func toPublicTraces(traces []db.Trace) []PublicTrace {
	out := make([]PublicTrace, 0, len(traces))
	for _, trace := range traces {
		out = append(out, toPublicTrace(trace))
	}
	return out
}

// PublicUsage is Langfuse's token-usage shape.
type PublicUsage struct {
	Input  int64  `json:"input"`
	Output int64  `json:"output"`
	Total  int64  `json:"total"`
	Unit   string `json:"unit"`
}

// PublicObservation is an observation in Langfuse's response shape.
type PublicObservation struct {
	ID                  string          `json:"id"`
	TraceID             string          `json:"traceId"`
	ProjectID           string          `json:"projectId"`
	Type                string          `json:"type"`
	Name                string          `json:"name,omitempty"`
	StartTime           string          `json:"startTime"`
	EndTime             string          `json:"endTime,omitempty"`
	CompletionStartTime string          `json:"completionStartTime,omitempty"`
	Input               json.RawMessage `json:"input,omitempty"`
	Output              json.RawMessage `json:"output,omitempty"`
	Metadata            json.RawMessage `json:"metadata,omitempty"`
	Model               string          `json:"model,omitempty"`
	ModelParameters     json.RawMessage `json:"modelParameters,omitempty"`
	Usage               PublicUsage     `json:"usage"`
	UsageDetails        json.RawMessage `json:"usageDetails,omitempty"`
	CostDetails         json.RawMessage `json:"costDetails,omitempty"`
	CalculatedTotalCost *float64        `json:"calculatedTotalCost,omitempty"`
	Level               string          `json:"level"`
	Status              string          `json:"status"`
	StatusMessage       string          `json:"statusMessage,omitempty"`
	ParentObservationID string          `json:"parentObservationId,omitempty"`
	PromptID            string          `json:"promptId,omitempty"`
	PromptName          string          `json:"promptName,omitempty"`
	PromptVersion       *int32          `json:"promptVersion,omitempty"`
	Version             string          `json:"version,omitempty"`
	Environment         string          `json:"environment"`
	Latency             *float64        `json:"latency,omitempty"`
}

// toPublicObservation converts a stored observation.
func toPublicObservation(obs db.Observation) PublicObservation {
	_, usage := services.NormalizeUsage(obs.TokenUsage)

	out := PublicObservation{
		ID:                  obs.ID,
		TraceID:             obs.TraceID,
		ProjectID:           obs.ProjectID,
		Type:                obs.Type,
		Name:                obs.Name.String,
		StartTime:           formatTime(obs.StartTime),
		EndTime:             formatTime(obs.EndTime),
		CompletionStartTime: formatTime(obs.CompletionStartTime),
		Input:               obs.Input,
		Output:              obs.Output,
		Metadata:            obs.Metadata,
		Model:               obs.Model.String,
		ModelParameters:     obs.ModelParameters,
		Usage: PublicUsage{
			Input:  usage.InputTokens,
			Output: usage.OutputTokens,
			Total:  usage.TotalTokens,
			Unit:   "TOKENS",
		},
		UsageDetails:        obs.UsageDetails,
		CostDetails:         obs.CostDetails,
		Level:               obs.Level,
		Status:              obs.Status,
		StatusMessage:       obs.StatusMessage.String,
		ParentObservationID: obs.ParentObservationID.String,
		PromptID:            obs.PromptID.String,
		PromptName:          obs.PromptName.String,
		Version:             obs.Version.String,
		Environment:         obs.Environment,
	}

	if obs.Cost.Valid {
		cost := obs.Cost.Float64
		out.CalculatedTotalCost = &cost
	}
	if obs.PromptVersion.Valid {
		version := obs.PromptVersion.Int32
		out.PromptVersion = &version
	}
	if latency, ok := durationSeconds(obs.StartTime, obs.EndTime); ok {
		out.Latency = &latency
	}

	return out
}

func toPublicObservations(observations []db.Observation) []PublicObservation {
	out := make([]PublicObservation, 0, len(observations))
	for _, obs := range observations {
		out = append(out, toPublicObservation(obs))
	}
	return out
}

// PublicScore is a score in Langfuse's response shape.
type PublicScore struct {
	ID            string          `json:"id"`
	TraceID       string          `json:"traceId,omitempty"`
	ObservationID string          `json:"observationId,omitempty"`
	SessionID     string          `json:"sessionId,omitempty"`
	DatasetRunID  string          `json:"datasetRunId,omitempty"`
	ProjectID     string          `json:"projectId"`
	Name          string          `json:"name"`
	Value         *float64        `json:"value,omitempty"`
	StringValue   string          `json:"stringValue,omitempty"`
	DataType      string          `json:"dataType"`
	Source        string          `json:"source"`
	Comment       string          `json:"comment,omitempty"`
	ConfigID      string          `json:"configId,omitempty"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
	Environment   string          `json:"environment"`
	CreatedAt     string          `json:"createdAt"`
}

// toPublicScore converts a stored score.
func toPublicScore(score db.Score) PublicScore {
	out := PublicScore{
		ID:            score.ID,
		TraceID:       score.TraceID.String,
		ObservationID: score.ObservationID.String,
		SessionID:     score.SessionID.String,
		DatasetRunID:  score.DatasetRunID.String,
		ProjectID:     score.ProjectID,
		Name:          score.Name,
		StringValue:   score.StringValue.String,
		DataType:      score.DataType,
		Source:        score.Source,
		Comment:       score.Comment.String,
		ConfigID:      score.ConfigID.String,
		Metadata:      score.Metadata,
		Environment:   score.Environment,
		CreatedAt:     formatTime(score.CreatedAt),
	}

	if score.Value.Valid {
		value := score.Value.Float64
		out.Value = &value
	}

	return out
}

func toPublicScores(scores []db.Score) []PublicScore {
	out := make([]PublicScore, 0, len(scores))
	for _, score := range scores {
		out = append(out, toPublicScore(score))
	}
	return out
}

// PublicSession is a session in Langfuse's response shape.
type PublicSession struct {
	ID          string        `json:"id"`
	ProjectID   string        `json:"projectId"`
	CreatedAt   string        `json:"createdAt"`
	Environment string        `json:"environment"`
	Bookmarked  bool          `json:"bookmarked"`
	Public      bool          `json:"public"`
	TraceCount  int64         `json:"traceCount"`
	TotalCost   float64       `json:"totalCost"`
	Traces      []PublicTrace `json:"traces,omitempty"`
}

// toPublicSession converts a session listing row.
func toPublicSession(row db.ListSessionsRow) PublicSession {
	return PublicSession{
		ID:          row.ID,
		ProjectID:   row.ProjectID,
		CreatedAt:   formatTime(row.CreatedAt),
		Environment: row.Environment,
		Bookmarked:  row.Bookmarked,
		Public:      row.Public,
		TraceCount:  row.TraceCount,
		TotalCost:   row.TotalCost,
	}
}

func toPublicSessions(rows []db.ListSessionsRow) []PublicSession {
	out := make([]PublicSession, 0, len(rows))
	for _, row := range rows {
		out = append(out, toPublicSession(row))
	}
	return out
}

// formatTime renders a timestamp as RFC 3339 in UTC, or "" when NULL.
func formatTime(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.UTC().Format(time.RFC3339Nano)
}

// durationSeconds returns the seconds between two timestamps, and whether both
// were present.
func durationSeconds(start, end pgtype.Timestamptz) (float64, bool) {
	if !start.Valid || !end.Valid {
		return 0, false
	}
	return end.Time.Sub(start.Time).Seconds(), true
}

// PublicDatasetItem is a dataset item in Langfuse's response shape.
type PublicDatasetItem struct {
	ID                  string          `json:"id"`
	DatasetID           string          `json:"datasetId"`
	DatasetName         string          `json:"datasetName,omitempty"`
	Input               json.RawMessage `json:"input,omitempty"`
	ExpectedOutput      json.RawMessage `json:"expectedOutput,omitempty"`
	Metadata            json.RawMessage `json:"metadata,omitempty"`
	SourceTraceID       string          `json:"sourceTraceId,omitempty"`
	SourceObservationID string          `json:"sourceObservationId,omitempty"`
	Status              string          `json:"status"`
	CreatedAt           string          `json:"createdAt"`
}

// toPublicDatasetItem converts a stored dataset item.
func toPublicDatasetItem(item db.DatasetItem, datasetName string) PublicDatasetItem {
	return PublicDatasetItem{
		ID:                  item.ID,
		DatasetID:           item.DatasetID,
		DatasetName:         datasetName,
		Input:               item.Input,
		ExpectedOutput:      item.ExpectedOutput,
		Metadata:            item.Metadata,
		SourceTraceID:       item.SourceTraceID.String,
		SourceObservationID: item.SourceObservationID.String,
		Status:              item.Status,
		CreatedAt:           formatTime(item.CreatedAt),
	}
}

func toPublicDatasetItems(items []db.DatasetItem, datasetName string) []PublicDatasetItem {
	out := make([]PublicDatasetItem, 0, len(items))
	for _, item := range items {
		out = append(out, toPublicDatasetItem(item, datasetName))
	}
	return out
}

// PublicDataset is a dataset in Langfuse's response shape.
type PublicDataset struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	ProjectID   string              `json:"projectId"`
	CreatedAt   string              `json:"createdAt"`
	Items       []PublicDatasetItem `json:"items,omitempty"`
}

// toPublicDataset converts a stored dataset.
func toPublicDataset(dataset db.Dataset) PublicDataset {
	return PublicDataset{
		ID:          dataset.ID,
		Name:        dataset.Name,
		Description: dataset.Description.String,
		ProjectID:   dataset.ProjectID,
		CreatedAt:   formatTime(dataset.CreatedAt),
	}
}

func toPublicDatasets(datasets []db.Dataset) []PublicDataset {
	out := make([]PublicDataset, 0, len(datasets))
	for _, dataset := range datasets {
		out = append(out, toPublicDataset(dataset))
	}
	return out
}
