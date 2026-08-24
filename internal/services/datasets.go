package services

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/langfuse-light/langfuse-light/internal/db"
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

// GetDataset returns a dataset by ID.
func (s *DatasetService) GetDataset(ctx context.Context, id string) (db.Dataset, error) {
	dataset, err := s.queries.GetDatasetByID(ctx, id)
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

// DeleteDataset deletes a dataset by ID.
func (s *DatasetService) DeleteDataset(ctx context.Context, id string) error {
	if err := s.queries.DeleteDataset(ctx, id); err != nil {
		return fmt.Errorf("deleting dataset: %w", err)
	}
	return nil
}

// CreateDatasetItemRequest is the request body for creating a dataset item.
type CreateDatasetItemRequest struct {
	Input          json.RawMessage `json:"input"`
	ExpectedOutput json.RawMessage `json:"expected_output"`
	Metadata       json.RawMessage `json:"metadata"`
	SourceTraceID  string          `json:"source_trace_id"`
}

// CreateDatasetItem adds a new item to a dataset.
func (s *DatasetService) CreateDatasetItem(ctx context.Context, datasetID string, req CreateDatasetItemRequest) (db.DatasetItem, error) {
	if len(req.Input) == 0 {
		return db.DatasetItem{}, fmt.Errorf("input is required")
	}

	item, err := s.queries.CreateDatasetItem(ctx, db.CreateDatasetItemParams{
		DatasetID:      datasetID,
		Input:          req.Input,
		ExpectedOutput: req.ExpectedOutput,
		Metadata:       req.Metadata,
		SourceTraceID:  pgtype.Text{String: req.SourceTraceID, Valid: req.SourceTraceID != ""},
	})
	if err != nil {
		return db.DatasetItem{}, fmt.Errorf("creating dataset item: %w", err)
	}

	return item, nil
}

// ListDatasetItems returns all items for a dataset.
func (s *DatasetService) ListDatasetItems(ctx context.Context, datasetID string) ([]db.DatasetItem, error) {
	items, err := s.queries.GetDatasetItemsByDatasetID(ctx, datasetID)
	if err != nil {
		return nil, fmt.Errorf("listing dataset items: %w", err)
	}
	return items, nil
}

// ImportItemsJSON imports dataset items from a JSON array.
func (s *DatasetService) ImportItemsJSON(ctx context.Context, datasetID string, data io.Reader) ([]db.DatasetItem, error) {
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
func (s *DatasetService) ImportItemsCSV(ctx context.Context, datasetID string, data io.Reader) ([]db.DatasetItem, error) {
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
func (s *DatasetService) ExportItemsJSON(ctx context.Context, datasetID string) ([]DatasetItemExport, error) {
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
type CreateDatasetRunRequest struct {
	Name string `json:"name"`
}

// CreateDatasetRun creates a new run for a dataset.
func (s *DatasetService) CreateDatasetRun(ctx context.Context, datasetID string, req CreateDatasetRunRequest) (db.DatasetRun, error) {
	if req.Name == "" {
		return db.DatasetRun{}, fmt.Errorf("name is required")
	}

	run, err := s.queries.CreateDatasetRun(ctx, db.CreateDatasetRunParams{
		DatasetID: datasetID,
		Name:      req.Name,
	})
	if err != nil {
		return db.DatasetRun{}, fmt.Errorf("creating dataset run: %w", err)
	}

	return run, nil
}

// ListDatasetRuns returns all runs for a dataset.
func (s *DatasetService) ListDatasetRuns(ctx context.Context, datasetID string) ([]db.DatasetRun, error) {
	runs, err := s.queries.GetDatasetRunsByDatasetID(ctx, datasetID)
	if err != nil {
		return nil, fmt.Errorf("listing dataset runs: %w", err)
	}
	return runs, nil
}

// GetDatasetRun returns a run by ID.
func (s *DatasetService) GetDatasetRun(ctx context.Context, id string) (db.DatasetRun, error) {
	run, err := s.queries.GetDatasetRunByID(ctx, id)
	if err != nil {
		return db.DatasetRun{}, fmt.Errorf("getting dataset run: %w", err)
	}
	return run, nil
}

// CreateDatasetRunItemRequest is the request body for creating a run item.
type CreateDatasetRunItemRequest struct {
	DatasetItemID string `json:"dataset_item_id"`
	ObservationID string `json:"observation_id"`
	ScoreID       string `json:"score_id"`
}

// CreateDatasetRunItem adds an item result to a dataset run.
func (s *DatasetService) CreateDatasetRunItem(ctx context.Context, runID string, req CreateDatasetRunItemRequest) (db.DatasetRunItem, error) {
	if req.DatasetItemID == "" {
		return db.DatasetRunItem{}, fmt.Errorf("dataset_item_id is required")
	}

	runItem, err := s.queries.CreateDatasetRunItem(ctx, db.CreateDatasetRunItemParams{
		DatasetRunID:  runID,
		DatasetItemID: req.DatasetItemID,
		ObservationID: pgtype.Text{String: req.ObservationID, Valid: req.ObservationID != ""},
		ScoreID:       pgtype.Text{String: req.ScoreID, Valid: req.ScoreID != ""},
	})
	if err != nil {
		return db.DatasetRunItem{}, fmt.Errorf("creating dataset run item: %w", err)
	}

	return runItem, nil
}

// ListDatasetRunItems returns all items for a run.
func (s *DatasetService) ListDatasetRunItems(ctx context.Context, runID string) ([]db.DatasetRunItem, error) {
	items, err := s.queries.GetDatasetRunItemsByRunID(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("listing dataset run items: %w", err)
	}
	return items, nil
}

// ExportRunResults exports the results of a dataset run with item and score details.
func (s *DatasetService) ExportRunResults(ctx context.Context, runID string) ([]RunResultExport, error) {
	runItems, err := s.queries.GetDatasetRunItemsByRunID(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("listing run items: %w", err)
	}

	var results []RunResultExport
	for _, ri := range runItems {
		export := RunResultExport{
			RunItemID:     ri.ID,
			DatasetItemID: ri.DatasetItemID,
			ObservationID: ri.ObservationID.String,
			ScoreID:       ri.ScoreID.String,
		}

		item, err := s.queries.GetDatasetItemByID(ctx, ri.DatasetItemID)
		if err == nil {
			export.Input = item.Input
			export.ExpectedOutput = item.ExpectedOutput
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

// RunResultExport represents an exported run result.
type RunResultExport struct {
	RunItemID      string          `json:"run_item_id"`
	DatasetItemID  string          `json:"dataset_item_id"`
	Input          json.RawMessage `json:"input"`
	ExpectedOutput json.RawMessage `json:"expected_output"`
	ObservationID  string          `json:"observation_id,omitempty"`
	ScoreID        string          `json:"score_id,omitempty"`
	Score          *ScoreExport    `json:"score,omitempty"`
}

// ScoreExport represents score data for export.
type ScoreExport struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}
