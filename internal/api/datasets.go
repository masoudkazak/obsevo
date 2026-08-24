package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/langfuse-light/langfuse-light/internal/services"
)

// DatasetHandler handles dataset HTTP requests.
type DatasetHandler struct {
	datasetService *services.DatasetService
}

// NewDatasetHandler creates a new dataset handler.
func NewDatasetHandler(datasetService *services.DatasetService) *DatasetHandler {
	return &DatasetHandler{datasetService: datasetService}
}

// CreateDataset handles POST /api/datasets.
func (h *DatasetHandler) CreateDataset(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	var req services.CreateDatasetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	dataset, err := h.datasetService.CreateDataset(r.Context(), projectID, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create dataset: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, dataset)
}

// ListDatasets handles GET /api/datasets.
func (h *DatasetHandler) ListDatasets(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
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
	id := chi.URLParam(r, "id")

	dataset, err := h.datasetService.GetDataset(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "dataset not found")
		return
	}

	writeJSON(w, http.StatusOK, dataset)
}

// DeleteDataset handles DELETE /api/datasets/{id}.
func (h *DatasetHandler) DeleteDataset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.datasetService.DeleteDataset(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete dataset: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// CreateDatasetItem handles POST /api/datasets/{id}/items.
func (h *DatasetHandler) CreateDatasetItem(w http.ResponseWriter, r *http.Request) {
	datasetID := chi.URLParam(r, "id")

	var req services.CreateDatasetItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Input) == 0 {
		writeError(w, http.StatusBadRequest, "input is required")
		return
	}

	item, err := h.datasetService.CreateDatasetItem(r.Context(), datasetID, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create dataset item: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, item)
}

// ListDatasetItems handles GET /api/datasets/{id}/items.
func (h *DatasetHandler) ListDatasetItems(w http.ResponseWriter, r *http.Request) {
	datasetID := chi.URLParam(r, "id")

	items, err := h.datasetService.ListDatasetItems(r.Context(), datasetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list dataset items: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, items)
}

// ImportDatasetItems handles POST /api/datasets/{id}/import.
func (h *DatasetHandler) ImportDatasetItems(w http.ResponseWriter, r *http.Request) {
	datasetID := chi.URLParam(r, "id")

	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/csv") || strings.Contains(contentType, "application/csv") {
		items, err := h.datasetService.ImportItemsCSV(r.Context(), datasetID, r.Body)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to import CSV: "+err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"imported": len(items),
			"items":    items,
		})
		return
	}

	items, err := h.datasetService.ImportItemsJSON(r.Context(), datasetID, r.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to import JSON: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"imported": len(items),
		"items":    items,
	})
}

// ExportDatasetItems handles GET /api/datasets/{id}/export.
func (h *DatasetHandler) ExportDatasetItems(w http.ResponseWriter, r *http.Request) {
	datasetID := chi.URLParam(r, "id")

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	items, err := h.datasetService.ExportItemsJSON(r.Context(), datasetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to export dataset items: "+err.Error())
		return
	}

	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=dataset_items.csv")
		w.WriteHeader(http.StatusOK)

		w.Write([]byte("id,input,expected_output,metadata\n"))
		for _, item := range items {
			w.Write([]byte(escapeCSV(item.ID) + ","))
			w.Write([]byte(escapeCSV(string(item.Input)) + ","))
			w.Write([]byte(escapeCSV(string(item.ExpectedOutput)) + ","))
			w.Write([]byte(escapeCSV(string(item.Metadata)) + "\n"))
		}
		return
	}

	writeJSON(w, http.StatusOK, items)
}

func escapeCSV(s string) string {
	if strings.ContainsAny(s, ",\"") {
		return "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\""
	}
	return s
}

// CreateDatasetRun handles POST /api/datasets/{id}/runs.
func (h *DatasetHandler) CreateDatasetRun(w http.ResponseWriter, r *http.Request) {
	datasetID := chi.URLParam(r, "id")

	var req services.CreateDatasetRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	run, err := h.datasetService.CreateDatasetRun(r.Context(), datasetID, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create dataset run: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, run)
}

// ListDatasetRuns handles GET /api/datasets/{id}/runs.
func (h *DatasetHandler) ListDatasetRuns(w http.ResponseWriter, r *http.Request) {
	datasetID := chi.URLParam(r, "id")

	runs, err := h.datasetService.ListDatasetRuns(r.Context(), datasetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list dataset runs: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, runs)
}

// GetDatasetRun handles GET /api/datasets/{id}/runs/{runId}.
func (h *DatasetHandler) GetDatasetRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runId")

	run, err := h.datasetService.GetDatasetRun(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusNotFound, "dataset run not found")
		return
	}

	writeJSON(w, http.StatusOK, run)
}

// CreateDatasetRunItem handles POST /api/datasets/{id}/runs/{runId}/items.
func (h *DatasetHandler) CreateDatasetRunItem(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runId")

	var req services.CreateDatasetRunItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.DatasetItemID == "" {
		writeError(w, http.StatusBadRequest, "dataset_item_id is required")
		return
	}

	runItem, err := h.datasetService.CreateDatasetRunItem(r.Context(), runID, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create dataset run item: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, runItem)
}

// ListDatasetRunItems handles GET /api/datasets/{id}/runs/{runId}/items.
func (h *DatasetHandler) ListDatasetRunItems(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runId")

	items, err := h.datasetService.ListDatasetRunItems(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list dataset run items: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, items)
}

// ExportDatasetRunResults handles GET /api/datasets/{id}/runs/{runId}/export.
func (h *DatasetHandler) ExportDatasetRunResults(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runId")

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	results, err := h.datasetService.ExportRunResults(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to export run results: "+err.Error())
		return
	}

	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=run_results.csv")
		w.WriteHeader(http.StatusOK)

		w.Write([]byte("run_item_id,dataset_item_id,input,expected_output,observation_id,score_id,score_name,score_value\n"))
		for _, r := range results {
			w.Write([]byte(escapeCSV(r.RunItemID) + ","))
			w.Write([]byte(escapeCSV(r.DatasetItemID) + ","))
			w.Write([]byte(escapeCSV(string(r.Input)) + ","))
			w.Write([]byte(escapeCSV(string(r.ExpectedOutput)) + ","))
			w.Write([]byte(escapeCSV(r.ObservationID) + ","))
			w.Write([]byte(escapeCSV(r.ScoreID) + ","))
			if r.Score != nil {
				w.Write([]byte(escapeCSV(r.Score.Name) + ","))
				w.Write([]byte(json.Number(json.Number(floatToString(r.Score.Value))).String() + "\n"))
			} else {
				w.Write([]byte(",\n"))
			}
		}
		return
	}

	writeJSON(w, http.StatusOK, results)
}

func floatToString(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}

// RegisterRoutes registers dataset routes.
func (h *DatasetHandler) RegisterRoutes(r chi.Router) {
	r.Post("/datasets", h.CreateDataset)
	r.Get("/datasets", h.ListDatasets)
	r.Get("/datasets/{id}", h.GetDataset)
	r.Delete("/datasets/{id}", h.DeleteDataset)
	r.Post("/datasets/{id}/items", h.CreateDatasetItem)
	r.Get("/datasets/{id}/items", h.ListDatasetItems)
	r.Post("/datasets/{id}/import", h.ImportDatasetItems)
	r.Get("/datasets/{id}/export", h.ExportDatasetItems)
	r.Post("/datasets/{id}/runs", h.CreateDatasetRun)
	r.Get("/datasets/{id}/runs", h.ListDatasetRuns)
	r.Get("/datasets/{id}/runs/{runId}", h.GetDatasetRun)
	r.Post("/datasets/{id}/runs/{runId}/items", h.CreateDatasetRunItem)
	r.Get("/datasets/{id}/runs/{runId}/items", h.ListDatasetRunItems)
	r.Get("/datasets/{id}/runs/{runId}/export", h.ExportDatasetRunResults)
}
