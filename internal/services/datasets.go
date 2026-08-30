package services

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/obsevo/obsevo/internal/db"
)

// DatasetService handles dataset business logic.
type DatasetService struct {
	queries *db.Queries
}

// NewDatasetService creates a new dataset service.
func NewDatasetService(queries *db.Queries) *DatasetService {
	return &DatasetService{queries: queries}
}

// CreateDatasetRequest is the request body for creating a dataset.
type CreateDatasetRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CreateDataset creates a new dataset for a project.
func (s *DatasetService) CreateDataset(ctx context.Context, projectID string, req CreateDatasetRequest) (db.Dataset, error) {
	if req.Name == "" {
		return db.Dataset{}, fmt.Errorf("name is required")
	}

	dataset, err := s.queries.CreateDataset(ctx, db.CreateDatasetParams{
		ProjectID:   projectID,
		Name:        req.Name,
		Description: pgtype.Text{String: req.Description, Valid: req.Description != ""},
	})
	if err != nil {
		return db.Dataset{}, fmt.Errorf("creating dataset: %w", err)
	}

	return dataset, nil
}

// GetDataset returns a dataset by ID, scoped to a project.
func (s *DatasetService) GetDataset(ctx context.Context, projectID, id string) (db.Dataset, error) {
	dataset, err := s.queries.GetDatasetByIDAndProject(ctx, db.GetDatasetByIDAndProjectParams{
		ID:        id,
		ProjectID: projectID,
	})
	if err != nil {
		return db.Dataset{}, fmt.Errorf("getting dataset: %w", err)
	}
	return dataset, nil
}

// GetDatasetByName returns a dataset by name, scoped to a project.
func (s *DatasetService) GetDatasetByName(ctx context.Context, projectID, name string) (db.Dataset, error) {
	dataset, err := s.queries.GetDatasetByNameAndProject(ctx, db.GetDatasetByNameAndProjectParams{
		Name:      name,
		ProjectID: projectID,
	})
	if err != nil {
		return db.Dataset{}, fmt.Errorf("getting dataset: %w", err)
	}
	return dataset, nil
}

// ListDatasets returns all datasets for a project.
func (s *DatasetService) ListDatasets(ctx context.Context, projectID string) ([]db.Dataset, error) {
	datasets, err := s.queries.GetDatasetsByProjectID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("listing datasets: %w", err)
	}
	return datasets, nil
}

// DeleteDataset deletes a dataset, scoped to a project.
func (s *DatasetService) DeleteDataset(ctx context.Context, projectID, id string) error {
	err := s.queries.DeleteDatasetInProject(ctx, db.DeleteDatasetInProjectParams{
		ID:        id,
		ProjectID: projectID,
	})
	if err != nil {
		return fmt.Errorf("deleting dataset: %w", err)
	}
	return nil
}

// requireDataset resolves a dataset within a project, so every nested route
// (items, runs, run items) is authorized by the dataset's own project.
func (s *DatasetService) requireDataset(ctx context.Context, projectID, datasetID string) (db.Dataset, error) {
	return s.GetDataset(ctx, projectID, datasetID)
}

// CreateDatasetItemRequest is the request body for creating a dataset item.
type CreateDatasetItemRequest struct {
	ID                  string          `json:"id"`
	Input               json.RawMessage `json:"input"`
	ExpectedOutput      json.RawMessage `json:"expected_output"`
	Metadata            json.RawMessage `json:"metadata"`
	SourceTraceID       string          `json:"source_trace_id"`
	SourceObservationID string          `json:"source_observation_id"`
	Status              string          `json:"status"`
}

// CreateDatasetItem adds an item to a dataset, or updates it when an id is
// supplied — Langfuse's dataset-item endpoint is an upsert.
func (s *DatasetService) CreateDatasetItem(ctx context.Context, projectID, datasetID string, req CreateDatasetItemRequest) (db.DatasetItem, error) {
	if _, err := s.requireDataset(ctx, projectID, datasetID); err != nil {
		return db.DatasetItem{}, err
	}
	if len(req.Input) == 0 {
		return db.DatasetItem{}, fmt.Errorf("input is required")
	}

	status := strings.ToUpper(req.Status)
	if status != "ARCHIVED" {
		status = "ACTIVE"
	}

	item, err := s.queries.CreateDatasetItemFull(ctx, db.CreateDatasetItemFullParams{
		ID:                  req.ID,
		DatasetID:           datasetID,
		Input:               req.Input,
		ExpectedOutput:      req.ExpectedOutput,
		Metadata:            req.Metadata,
		SourceTraceID:       req.SourceTraceID,
		SourceObservationID: req.SourceObservationID,
		Status:              status,
	})
	if err != nil {
		// The upsert is guarded to its own dataset, so a conflicting id from
		// elsewhere matches no row rather than overwriting another dataset's item.
		if errors.Is(err, pgx.ErrNoRows) {
			return db.DatasetItem{}, fmt.Errorf("dataset item id %q already exists in another dataset", req.ID)
		}
		return db.DatasetItem{}, fmt.Errorf("creating dataset item: %w", err)
	}

	return item, nil
}

// ListDatasetItems returns all items for a dataset in a project.
func (s *DatasetService) ListDatasetItems(ctx context.Context, projectID, datasetID string) ([]db.DatasetItem, error) {
	if _, err := s.requireDataset(ctx, projectID, datasetID); err != nil {
		return nil, err
	}
	items, err := s.queries.GetDatasetItemsByDatasetID(ctx, datasetID)
	if err != nil {
		return nil, fmt.Errorf("listing dataset items: %w", err)
	}
	return items, nil
}

// DeleteDatasetItem removes one item from a dataset.
func (s *DatasetService) DeleteDatasetItem(ctx context.Context, projectID, datasetID, itemID string) error {
	if _, err := s.requireDataset(ctx, projectID, datasetID); err != nil {
		return err
	}
	err := s.queries.DeleteDatasetItem(ctx, db.DeleteDatasetItemParams{
		ID:        itemID,
		DatasetID: datasetID,
	})
	if err != nil {
		return fmt.Errorf("deleting dataset item: %w", err)
	}
	return nil
}

// AddTracesRequest selects production traces to turn into dataset items.
type AddTracesRequest struct {
	TraceIDs []string `json:"trace_ids"`

	// UseOutputAsExpected copies each trace's output into the item's expected
	// output, which is the usual way to build a regression set from traffic
	// that was already judged acceptable.
	UseOutputAsExpected bool `json:"use_output_as_expected"`
}

// AddTracesToDataset converts production traces into dataset items. This is the
// "promote real traffic into a test set" workflow: pick traces, create items,
// then run experiments against them.
func (s *DatasetService) AddTracesToDataset(ctx context.Context, projectID, datasetID string, req AddTracesRequest) ([]db.DatasetItem, error) {
	if _, err := s.requireDataset(ctx, projectID, datasetID); err != nil {
		return nil, err
	}
	if len(req.TraceIDs) == 0 {
		return nil, fmt.Errorf("trace_ids must not be empty")
	}

	created := make([]db.DatasetItem, 0, len(req.TraceIDs))
	for _, traceID := range req.TraceIDs {
		trace, err := s.queries.GetTraceByIDAndProject(ctx, db.GetTraceByIDAndProjectParams{
			ID:        traceID,
			ProjectID: projectID,
		})
		if err != nil {
			return created, fmt.Errorf("loading trace %s: %w", traceID, err)
		}
		if len(trace.Input) == 0 {
			// A trace with no input cannot seed a test case.
			continue
		}

		params := db.CreateDatasetItemFullParams{
			DatasetID:     datasetID,
			Input:         trace.Input,
			Metadata:      trace.Metadata,
			SourceTraceID: traceID,
			Status:        "ACTIVE",
		}
		if req.UseOutputAsExpected {
			params.ExpectedOutput = trace.Output
		}

		item, err := s.queries.CreateDatasetItemFull(ctx, params)
		if err != nil {
			return created, fmt.Errorf("creating dataset item from trace %s: %w", traceID, err)
		}
		created = append(created, item)
	}

	return created, nil
}

// ImportItemsJSON imports dataset items from a JSON array.
func (s *DatasetService) ImportItemsJSON(ctx context.Context, projectID, datasetID string, data io.Reader) ([]db.DatasetItem, error) {
	if _, err := s.requireDataset(ctx, projectID, datasetID); err != nil {
		return nil, err
	}

	var items []struct {
		Input          json.RawMessage `json:"input"`
		ExpectedOutput json.RawMessage `json:"expected_output"`
		Metadata       json.RawMessage `json:"metadata"`
	}

	if err := json.NewDecoder(data).Decode(&items); err != nil {
		return nil, fmt.Errorf("decoding JSON: %w", err)
	}

	var created []db.DatasetItem
	for _, item := range items {
		if len(item.Input) == 0 {
			continue
		}
		result, err := s.queries.CreateDatasetItem(ctx, db.CreateDatasetItemParams{
			DatasetID:      datasetID,
			Input:          item.Input,
			ExpectedOutput: item.ExpectedOutput,
			Metadata:       item.Metadata,
		})
		if err != nil {
			return created, fmt.Errorf("creating dataset item: %w", err)
		}
		created = append(created, result)
	}

	return created, nil
}

// ImportItemsCSV imports dataset items from a CSV reader.
// Expected format: input,expected_output,metadata (header row optional).
func (s *DatasetService) ImportItemsCSV(ctx context.Context, projectID, datasetID string, data io.Reader) ([]db.DatasetItem, error) {
	if _, err := s.requireDataset(ctx, projectID, datasetID); err != nil {
		return nil, err
	}

	reader := csv.NewReader(data)

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading CSV: %w", err)
	}

	startIdx := 0
	if len(records) > 0 {
		first := strings.ToLower(strings.TrimSpace(records[0][0]))
		if first == "input" || first == "expected_output" || first == "metadata" {
			startIdx = 1
		}
	}

	var created []db.DatasetItem
	for _, record := range records[startIdx:] {
		if len(record) == 0 || strings.TrimSpace(record[0]) == "" {
			continue
		}

		item := db.CreateDatasetItemParams{
			DatasetID: datasetID,
			Input:     []byte(record[0]),
		}

		if len(record) > 1 && strings.TrimSpace(record[1]) != "" {
			item.ExpectedOutput = []byte(record[1])
		}
		if len(record) > 2 && strings.TrimSpace(record[2]) != "" {
			item.Metadata = []byte(record[2])
		}

		result, err := s.queries.CreateDatasetItem(ctx, item)
		if err != nil {
			return created, fmt.Errorf("creating dataset item: %w", err)
		}
		created = append(created, result)
	}

	return created, nil
}

// ExportItemsJSON exports all dataset items as a JSON array.
func (s *DatasetService) ExportItemsJSON(ctx context.Context, projectID, datasetID string) ([]DatasetItemExport, error) {
	if _, err := s.requireDataset(ctx, projectID, datasetID); err != nil {
		return nil, err
	}

	items, err := s.queries.GetDatasetItemsByDatasetID(ctx, datasetID)
	if err != nil {
		return nil, fmt.Errorf("listing dataset items: %w", err)
	}

	exports := make([]DatasetItemExport, len(items))
	for i, item := range items {
		exports[i] = DatasetItemExport{
			ID:             item.ID,
			Input:          item.Input,
			ExpectedOutput: item.ExpectedOutput,
			Metadata:       item.Metadata,
			SourceTraceID:  item.SourceTraceID.String,
		}
	}

	return exports, nil
}

// DatasetItemExport represents a dataset item for export.
type DatasetItemExport struct {
	ID             string          `json:"id"`
	Input          json.RawMessage `json:"input"`
	ExpectedOutput json.RawMessage `json:"expected_output"`
	Metadata       json.RawMessage `json:"metadata"`
	SourceTraceID  string          `json:"source_trace_id,omitempty"`
}

// CreateDatasetRunRequest is the request body for creating a dataset run.
//
// Metadata is where an experiment records the configuration under test — the
// prompt name and version, the model, and its parameters — so two runs can be
// compared and the difference attributed.
type CreateDatasetRunRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Metadata    json.RawMessage `json:"metadata"`
}

// CreateDatasetRun creates a new run for a dataset.
func (s *DatasetService) CreateDatasetRun(ctx context.Context, projectID, datasetID string, req CreateDatasetRunRequest) (db.DatasetRun, error) {
	if _, err := s.requireDataset(ctx, projectID, datasetID); err != nil {
		return db.DatasetRun{}, err
	}
	if req.Name == "" {
		return db.DatasetRun{}, fmt.Errorf("name is required")
	}

	run, err := s.queries.CreateDatasetRunFull(ctx, db.CreateDatasetRunFullParams{
		DatasetID:   datasetID,
		Name:        req.Name,
		Description: req.Description,
		Metadata:    req.Metadata,
	})
	if err != nil {
		return db.DatasetRun{}, fmt.Errorf("creating dataset run: %w", err)
	}

	return run, nil
}

// ListDatasetRuns returns all runs for a dataset in a project.
func (s *DatasetService) ListDatasetRuns(ctx context.Context, projectID, datasetID string) ([]db.DatasetRun, error) {
	if _, err := s.requireDataset(ctx, projectID, datasetID); err != nil {
		return nil, err
	}
	runs, err := s.queries.GetDatasetRunsByDatasetID(ctx, datasetID)
	if err != nil {
		return nil, fmt.Errorf("listing dataset runs: %w", err)
	}
	return runs, nil
}

// GetDatasetRun returns a run by ID within a dataset.
func (s *DatasetService) GetDatasetRun(ctx context.Context, projectID, datasetID, id string) (db.DatasetRun, error) {
	if _, err := s.requireDataset(ctx, projectID, datasetID); err != nil {
		return db.DatasetRun{}, err
	}
	run, err := s.queries.GetDatasetRunByIDAndDataset(ctx, db.GetDatasetRunByIDAndDatasetParams{
		ID:        id,
		DatasetID: datasetID,
	})
	if err != nil {
		return db.DatasetRun{}, fmt.Errorf("getting dataset run: %w", err)
	}
	return run, nil
}

// DeleteDatasetRun removes a run and its items.
func (s *DatasetService) DeleteDatasetRun(ctx context.Context, projectID, datasetID, id string) error {
	if _, err := s.requireDataset(ctx, projectID, datasetID); err != nil {
		return err
	}
	err := s.queries.DeleteDatasetRun(ctx, db.DeleteDatasetRunParams{
		ID:        id,
		DatasetID: datasetID,
	})
	if err != nil {
		return fmt.Errorf("deleting dataset run: %w", err)
	}
	return nil
}

// EnsureRunRequest names a run to resolve or create.
type EnsureRunRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Metadata    json.RawMessage `json:"metadata"`
}

// EnsureRunByName resolves a dataset run by name, creating it if absent.
//
// The Langfuse dataset-run-item endpoint identifies a run only by name and the
// item it is reporting on, so the dataset is derived from the item and the run
// is opened on first use.
func (s *DatasetService) EnsureRunByName(ctx context.Context, projectID, datasetItemID string, req EnsureRunRequest) (db.DatasetRun, error) {
	if req.Name == "" {
		return db.DatasetRun{}, fmt.Errorf("run name is required")
	}

	item, err := s.queries.GetDatasetItemByID(ctx, datasetItemID)
	if err != nil {
		return db.DatasetRun{}, fmt.Errorf("dataset item not found: %w", err)
	}
	if _, err := s.requireDataset(ctx, projectID, item.DatasetID); err != nil {
		return db.DatasetRun{}, err
	}

	existing, err := s.queries.GetDatasetRunByNameAndDataset(ctx, db.GetDatasetRunByNameAndDatasetParams{
		Name:      req.Name,
		DatasetID: item.DatasetID,
	})
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.DatasetRun{}, fmt.Errorf("looking up dataset run: %w", err)
	}

	run, err := s.queries.CreateDatasetRunFull(ctx, db.CreateDatasetRunFullParams{
		DatasetID:   item.DatasetID,
		Name:        req.Name,
		Description: req.Description,
		Metadata:    req.Metadata,
	})
	if err != nil {
		return db.DatasetRun{}, fmt.Errorf("creating dataset run: %w", err)
	}
	return run, nil
}

// CreateDatasetRunItemRequest is the request body for creating a run item.
// TraceID links the item to the trace produced when the configuration under
// test was executed against it; the run's cost and latency aggregates are
// derived from those traces.
type CreateDatasetRunItemRequest struct {
	DatasetItemID string `json:"dataset_item_id"`
	TraceID       string `json:"trace_id"`
	ObservationID string `json:"observation_id"`
	ScoreID       string `json:"score_id"`
}

// CreateDatasetRunItem records one item's result in a dataset run.
func (s *DatasetService) CreateDatasetRunItem(ctx context.Context, projectID, datasetID, runID string, req CreateDatasetRunItemRequest) (db.DatasetRunItem, error) {
	if _, err := s.GetDatasetRun(ctx, projectID, datasetID, runID); err != nil {
		return db.DatasetRunItem{}, err
	}
	if req.DatasetItemID == "" {
		return db.DatasetRunItem{}, fmt.Errorf("dataset_item_id is required")
	}

	// A run item may name a trace from another project only by mistake; check
	// before linking so a run cannot aggregate someone else's cost.
	if req.TraceID != "" {
		if _, err := s.queries.GetTraceByIDAndProject(ctx, db.GetTraceByIDAndProjectParams{
			ID:        req.TraceID,
			ProjectID: projectID,
		}); err != nil {
			return db.DatasetRunItem{}, fmt.Errorf("trace %s not found in this project", req.TraceID)
		}
	}

	runItem, err := s.queries.CreateDatasetRunItemFull(ctx, db.CreateDatasetRunItemFullParams{
		DatasetRunID:  runID,
		DatasetItemID: req.DatasetItemID,
		TraceID:       req.TraceID,
		ObservationID: req.ObservationID,
		ScoreID:       req.ScoreID,
	})
	if err != nil {
		return db.DatasetRunItem{}, fmt.Errorf("creating dataset run item: %w", err)
	}

	return runItem, nil
}

// ListDatasetRunItems returns all items for a run, with their inputs, expected
// outputs and the trace-derived cost and latency.
func (s *DatasetService) ListDatasetRunItems(ctx context.Context, projectID, datasetID, runID string) ([]db.GetDatasetRunItemsWithDetailRow, error) {
	if _, err := s.GetDatasetRun(ctx, projectID, datasetID, runID); err != nil {
		return nil, err
	}
	items, err := s.queries.GetDatasetRunItemsWithDetail(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("listing dataset run items: %w", err)
	}
	return items, nil
}

// ExportRunResults exports the results of a dataset run with item and score details.
func (s *DatasetService) ExportRunResults(ctx context.Context, projectID, datasetID, runID string) ([]RunResultExport, error) {
	if _, err := s.GetDatasetRun(ctx, projectID, datasetID, runID); err != nil {
		return nil, err
	}

	runItems, err := s.queries.GetDatasetRunItemsWithDetail(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("listing run items: %w", err)
	}

	results := make([]RunResultExport, 0, len(runItems))
	for _, ri := range runItems {
		export := RunResultExport{
			RunItemID:      ri.RunItemID,
			DatasetItemID:  ri.DatasetItemID,
			Input:          ri.ItemInput,
			ExpectedOutput: ri.ExpectedOutput,
			ActualOutput:   ri.TraceOutput,
			TraceID:        ri.TraceID.String,
			ObservationID:  ri.ObservationID.String,
			ScoreID:        ri.ScoreID.String,
			LatencySeconds: ri.LatencySeconds,
		}
		if ri.TotalCost.Valid {
			export.Cost = ri.TotalCost.Float64
		}

		if ri.ScoreID.Valid {
			score, err := s.queries.GetScoreByID(ctx, ri.ScoreID.String)
			if err == nil {
				export.Score = &ScoreExport{
					Name:  score.Name,
					Value: score.Value.Float64,
				}
			}
		}

		results = append(results, export)
	}

	return results, nil
}

// RunResultExport represents an exported run result: the test case, what the
// configuration under test actually produced, and what it cost.
type RunResultExport struct {
	RunItemID      string          `json:"run_item_id"`
	DatasetItemID  string          `json:"dataset_item_id"`
	Input          json.RawMessage `json:"input"`
	ExpectedOutput json.RawMessage `json:"expected_output"`
	ActualOutput   json.RawMessage `json:"actual_output,omitempty"`
	TraceID        string          `json:"trace_id,omitempty"`
	ObservationID  string          `json:"observation_id,omitempty"`
	ScoreID        string          `json:"score_id,omitempty"`
	Score          *ScoreExport    `json:"score,omitempty"`
	Cost           float64         `json:"cost"`
	LatencySeconds float64         `json:"latency_seconds"`
}

// ScoreExport represents score data for export.
type ScoreExport struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}
