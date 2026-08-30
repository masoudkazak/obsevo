package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/langfuse-light/langfuse-light/internal/services"
)

// ModelPriceHandler exposes per-model pricing used for cost tracking.
type ModelPriceHandler struct {
	costService *services.CostService
}

// NewModelPriceHandler creates a new model price handler.
func NewModelPriceHandler(costService *services.CostService) *ModelPriceHandler {
	return &ModelPriceHandler{costService: costService}
}

// ListModelPrices handles GET /api/model-prices.
// The result contains the project's own overrides plus the global defaults.
func (h *ModelPriceHandler) ListModelPrices(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	prices, err := h.costService.ListModelPrices(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list model prices")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"prices": prices})
}

// CreateModelPrice handles POST /api/model-prices.
func (h *ModelPriceHandler) CreateModelPrice(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	var req services.CreateModelPriceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	price, err := h.costService.CreateModelPrice(r.Context(), projectID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, price)
}

// DeleteModelPrice handles DELETE /api/model-prices/{id}.
func (h *ModelPriceHandler) DeleteModelPrice(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}

	if err := h.costService.DeleteModelPrice(r.Context(), projectID, chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete model price")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// RegisterRoutes registers model price routes.
func (h *ModelPriceHandler) RegisterRoutes(r chi.Router) {
	r.Get("/model-prices", h.ListModelPrices)
	r.Post("/model-prices", h.CreateModelPrice)
	r.Delete("/model-prices/{id}", h.DeleteModelPrice)
}
