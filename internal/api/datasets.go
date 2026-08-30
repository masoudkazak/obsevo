package api

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/langfuse-light/langfuse-light/internal/services"
)

// DatasetHandler handles dataset, dataset run and experiment HTTP requests.
type DatasetHandler struct {
	datasetService    *services.DatasetService
	experimentService *services.ExperimentService
}

// NewDatasetHandler creates a new dataset handler.
func NewDatasetHandler(datasetService *services.DatasetService, experimentService *services.ExperimentService) *DatasetHandler {
	return &DatasetHandler{datasetService: datasetService, experimentService: experimentService}
}

// --- datasets -------------------------------------------------------------

// CreateDataset handles POST /api/datasets.
func (h *DatasetHandler) CreateDataset(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	var req services.CreateDatasetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	dataset, err := h.datasetService.CreateDataset(r.Context(), projectID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, dataset)
}

// ListDatasets handles GET /api/datasets.
func (h *DatasetHandler) ListDatasets(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	datasets, err := h.datasetService.ListDatasets(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list datasets: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, datasets)
}

// GetDataset handles GET /api/datasets/{id}.
func (h *DatasetHandler) GetDataset(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	dataset, err := h.datasetService.GetDataset(r.Context(), projectID, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "dataset not found")
		return
	}

	writeJSON(w, http.StatusOK, dataset)
}

// DeleteDataset handles DELETE /api/datasets/{id}.
func (h *DatasetHandler) DeleteDataset(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	if err := h.datasetService.DeleteDataset(r.Context(), projectID, chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete dataset: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- dataset items --------------------------------------------------------

// CreateDatasetItem handles POST /api/datasets/{id}/items.
func (h *DatasetHandler) CreateDatasetItem(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	var req services.CreateDatasetItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	item, err := h.datasetService.CreateDatasetItem(r.Context(), projectID, chi.URLParam(r, "id"), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, item)
}

// ListDatasetItems handles GET /api/datasets/{id}/items.
func (h *DatasetHandler) ListDatasetItems(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	items, err := h.datasetService.ListDatasetItems(r.Context(), projectID, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "dataset not found")
		return
	}

	writeJSON(w, http.StatusOK, items)
}

// DeleteDatasetItem handles DELETE /api/datasets/{id}/items/{itemId}.
func (h *DatasetHandler) DeleteDatasetItem(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	err := h.datasetService.DeleteDatasetItem(r.Context(), projectID,
		chi.URLParam(r, "id"), chi.URLParam(r, "itemId"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete dataset item")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// AddTracesToDataset handles POST /api/datasets/{id}/from-traces — turns
// production traces into dataset items.
func (h *DatasetHandler) AddTracesToDataset(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	var req services.AddTracesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	items, err := h.datasetService.AddTracesToDataset(r.Context(), projectID, chi.URLParam(r, "id"), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"created": len(items),
		"items":   items,
	})
}

// --- import and export ----------------------------------------------------

// ImportDatasetItems handles POST /api/datasets/{id}/import.
func (h *DatasetHandler) ImportDatasetItems(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}
	datasetID := chi.URLParam(r, "id")

	// The body format is chosen by Content-Type; JSON is the default. The body
	// is read once, by whichever importer the content type selects.
	importItems := h.datasetService.ImportItemsJSON
	if strings.Contains(r.Header.Get("Content-Type"), "csv") {
		importItems = h.datasetService.ImportItemsCSV
	}

	items, err := importItems(r.Context(), projectID, datasetID, r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to import items: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"imported": len(items),
		"items":    items,
	})
}

// ExportDatasetItems handles GET /api/datasets/{id}/export.
func (h *DatasetHandler) ExportDatasetItems(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	items, err := h.datasetService.ExportItemsJSON(r.Context(), projectID, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "dataset not found")
		return
	}

	if r.URL.Query().Get("format") != "csv" {
		writeJSON(w, http.StatusOK, items)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=dataset_items.csv")
	w.WriteHeader(http.StatusOK)

	writer := csv.NewWriter(w)
	defer writer.Flush()
	_ = writer.Write([]string{"id", "input", "expected_output", "metadata", "source_trace_id"})
	for _, item := range items {
		_ = writer.Write([]string{
			item.ID,
			string(item.Input),
			string(item.ExpectedOutput),
			string(item.Metadata),
			item.SourceTraceID,
		})
	}
}

// --- dataset runs ---------------------------------------------------------

// CreateDatasetRun handles POST /api/datasets/{id}/runs.
func (h *DatasetHandler) CreateDatasetRun(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	var req services.CreateDatasetRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	run, err := h.datasetService.CreateDatasetRun(r.Context(), projectID, chi.URLParam(r, "id"), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, run)
}

// ListDatasetRuns handles GET /api/datasets/{id}/runs.
func (h *DatasetHandler) ListDatasetRuns(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	runs, err := h.datasetService.ListDatasetRuns(r.Context(), projectID, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "dataset not found")
		return
	}

	writeJSON(w, http.StatusOK, runs)
}

// GetDatasetRun handles GET /api/datasets/{id}/runs/{runId}.
func (h *DatasetHandler) GetDatasetRun(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	run, err := h.datasetService.GetDatasetRun(r.Context(), projectID,
		chi.URLParam(r, "id"), chi.URLParam(r, "runId"))
	if err != nil {
		writeError(w, http.StatusNotFound, "dataset run not found")
		return
	}

	writeJSON(w, http.StatusOK, run)
}

// DeleteDatasetRun handles DELETE /api/datasets/{id}/runs/{runId}.
func (h *DatasetHandler) DeleteDatasetRun(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	err := h.datasetService.DeleteDatasetRun(r.Context(), projectID,
		chi.URLParam(r, "id"), chi.URLParam(r, "runId"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete dataset run")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// CreateDatasetRunItem handles POST /api/datasets/{id}/runs/{runId}/items.
func (h *DatasetHandler) CreateDatasetRunItem(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	var req services.CreateDatasetRunItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	runItem, err := h.datasetService.CreateDatasetRunItem(r.Context(), projectID,
		chi.URLParam(r, "id"), chi.URLParam(r, "runId"), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, runItem)
}

// ListDatasetRunItems handles GET /api/datasets/{id}/runs/{runId}/items.
func (h *DatasetHandler) ListDatasetRunItems(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	items, err := h.datasetService.ListDatasetRunItems(r.Context(), projectID,
		chi.URLParam(r, "id"), chi.URLParam(r, "runId"))
	if err != nil {
		writeError(w, http.StatusNotFound, "dataset run not found")
		return
	}

	writeJSON(w, http.StatusOK, items)
}

// ExportDatasetRunResults handles GET /api/datasets/{id}/runs/{runId}/export.
func (h *DatasetHandler) ExportDatasetRunResults(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	results, err := h.datasetService.ExportRunResults(r.Context(), projectID,
		chi.URLParam(r, "id"), chi.URLParam(r, "runId"))
	if err != nil {
		writeError(w, http.StatusNotFound, "dataset run not found")
		return
	}

	if r.URL.Query().Get("format") != "csv" {
		writeJSON(w, http.StatusOK, results)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=run_results.csv")
	w.WriteHeader(http.StatusOK)

	writer := csv.NewWriter(w)
	defer writer.Flush()
	_ = writer.Write([]string{
		"run_item_id", "dataset_item_id", "input", "expected_output", "actual_output",
		"trace_id", "score_name", "score_value", "cost", "latency_seconds",
	})
	for _, result := range results {
		scoreName, scoreValue := "", ""
		if result.Score != nil {
			scoreName = result.Score.Name
			scoreValue = strconv.FormatFloat(result.Score.Value, 'f', -1, 64)
		}
		_ = writer.Write([]string{
			result.RunItemID,
			result.DatasetItemID,
			string(result.Input),
			string(result.ExpectedOutput),
			string(result.ActualOutput),
			result.TraceID,
			scoreName,
			scoreValue,
			strconv.FormatFloat(result.Cost, 'f', -1, 64),
			strconv.FormatFloat(result.LatencySeconds, 'f', -1, 64),
		})
	}
}

// --- experiments ----------------------------------------------------------

// ListRunSummaries handles GET /api/datasets/{id}/experiments — one aggregate
// row per run, which is the experiment comparison table.
func (h *DatasetHandler) ListRunSummaries(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}
	datasetID := chi.URLParam(r, "id")

	if _, err := h.datasetService.GetDataset(r.Context(), projectID, datasetID); err != nil {
		writeError(w, http.StatusNotFound, "dataset not found")
		return
	}

	summaries, err := h.experimentService.ListRunSummaries(r.Context(), datasetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to summarise runs: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"runs": summaries})
}

// GetRunSummary handles GET /api/datasets/{id}/runs/{runId}/summary.
func (h *DatasetHandler) GetRunSummary(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}
	datasetID := chi.URLParam(r, "id")

	if _, err := h.datasetService.GetDataset(r.Context(), projectID, datasetID); err != nil {
		writeError(w, http.StatusNotFound, "dataset not found")
		return
	}

	summary, err := h.experimentService.GetRunSummary(r.Context(), datasetID, chi.URLParam(r, "runId"))
	if err != nil {
		writeError(w, http.StatusNotFound, "dataset run not found")
		return
	}

	writeJSON(w, http.StatusOK, summary)
}

// CompareRuns handles GET /api/datasets/{id}/compare?baseline=&current= —
// the regression report between two runs of the same dataset.
func (h *DatasetHandler) CompareRuns(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}
	datasetID := chi.URLParam(r, "id")

	baseline := r.URL.Query().Get("baseline")
	current := r.URL.Query().Get("current")
	if baseline == "" || current == "" {
		writeError(w, http.StatusBadRequest, "baseline and current run ids are required")
		return
	}

	if _, err := h.datasetService.GetDataset(r.Context(), projectID, datasetID); err != nil {
		writeError(w, http.StatusNotFound, "dataset not found")
		return
	}

	comparison, err := h.experimentService.CompareRuns(r.Context(), datasetID, baseline, current)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, comparison)
}

// RegisterRoutes registers dataset, run and experiment routes.
func (h *DatasetHandler) RegisterRoutes(r chi.Router) {
	r.Post("/datasets", h.CreateDataset)
	r.Get("/datasets", h.ListDatasets)
	r.Get("/datasets/{id}", h.GetDataset)
	r.Delete("/datasets/{id}", h.DeleteDataset)

	r.Post("/datasets/{id}/items", h.CreateDatasetItem)
	r.Get("/datasets/{id}/items", h.ListDatasetItems)
	r.Delete("/datasets/{id}/items/{itemId}", h.DeleteDatasetItem)
	r.Post("/datasets/{id}/from-traces", h.AddTracesToDataset)

	r.Post("/datasets/{id}/import", h.ImportDatasetItems)
	r.Get("/datasets/{id}/export", h.ExportDatasetItems)

	r.Post("/datasets/{id}/runs", h.CreateDatasetRun)
	r.Get("/datasets/{id}/runs", h.ListDatasetRuns)
	r.Get("/datasets/{id}/runs/{runId}", h.GetDatasetRun)
	r.Delete("/datasets/{id}/runs/{runId}", h.DeleteDatasetRun)
	r.Post("/datasets/{id}/runs/{runId}/items", h.CreateDatasetRunItem)
	r.Get("/datasets/{id}/runs/{runId}/items", h.ListDatasetRunItems)
	r.Get("/datasets/{id}/runs/{runId}/export", h.ExportDatasetRunResults)
	r.Get("/datasets/{id}/runs/{runId}/summary", h.GetRunSummary)

	r.Get("/datasets/{id}/experiments", h.ListRunSummaries)
	r.Get("/datasets/{id}/compare", h.CompareRuns)
}
