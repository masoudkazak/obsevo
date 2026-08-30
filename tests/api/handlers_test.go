package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/langfuse-light/langfuse-light/internal/api"
	"github.com/langfuse-light/langfuse-light/internal/auth"
	"github.com/langfuse-light/langfuse-light/internal/services"
)

func setupTestRouter() *chi.Mux {
	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	return r
}

func TestHealthEndpoint(t *testing.T) {
	r := setupTestRouter()

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %q", resp["status"])
	}
}

func TestCreatePromptHandler_MissingProjectID(t *testing.T) {
	handler := api.NewPromptHandler(services.NewPromptService(nil))

	body := `{"name":"test","prompt":"hello"}`
	req := httptest.NewRequest("POST", "/prompts", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	handler.CreatePrompt(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestCreatePromptHandler_MissingName(t *testing.T) {
	handler := api.NewPromptHandler(services.NewPromptService(nil))

	body := `{"prompt":"hello"}`
	req := httptest.NewRequest("POST", "/prompts?project_id=proj-123", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	handler.CreatePrompt(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestCreatePromptHandler_MissingPrompt(t *testing.T) {
	handler := api.NewPromptHandler(services.NewPromptService(nil))

	body := `{"name":"test"}`
	req := httptest.NewRequest("POST", "/prompts?project_id=proj-123", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	handler.CreatePrompt(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestListPromptsHandler_MissingProjectID(t *testing.T) {
	handler := api.NewPromptHandler(services.NewPromptService(nil))

	req := httptest.NewRequest("GET", "/prompts", nil)
	w := httptest.NewRecorder()
	handler.ListPrompts(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestGetPromptByNameHandler_MissingProjectID(t *testing.T) {
	handler := api.NewPromptHandler(services.NewPromptService(nil))

	req := httptest.NewRequest("GET", "/prompts/test-prompt", nil)
	w := httptest.NewRecorder()
	handler.GetPromptByName(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestCreateScoreHandler_MissingTraceID(t *testing.T) {
	handler := api.NewEvaluationHandler(services.NewEvaluationService(nil))

	body := `{"name":"accuracy","value":0.9}`
	req := httptest.NewRequest("POST", "/scores", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	handler.CreateScore(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestCreateScoreHandler_MissingName(t *testing.T) {
	handler := api.NewEvaluationHandler(services.NewEvaluationService(nil))

	body := `{"trace_id":"trace-123","value":0.9}`
	req := httptest.NewRequest("POST", "/scores", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	handler.CreateScore(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestGetScoreAggregationsHandler_MissingProjectID(t *testing.T) {
	handler := api.NewEvaluationHandler(services.NewEvaluationService(nil))

	req := httptest.NewRequest("GET", "/scores/aggregation", nil)
	w := httptest.NewRecorder()
	handler.GetScoreAggregations(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestGetAnalyticsHandler_MissingProjectID(t *testing.T) {
	handler := api.NewEvaluationHandler(services.NewEvaluationService(nil))

	req := httptest.NewRequest("GET", "/analytics", nil)
	w := httptest.NewRecorder()
	handler.GetAnalytics(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestCreateDatasetHandler_MissingProjectID(t *testing.T) {
	handler := api.NewDatasetHandler(services.NewDatasetService(nil), services.NewExperimentService(nil))

	body := `{"name":"test-dataset"}`
	req := httptest.NewRequest("POST", "/datasets", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	handler.CreateDataset(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestCreateDatasetHandler_MissingName(t *testing.T) {
	handler := api.NewDatasetHandler(services.NewDatasetService(nil), services.NewExperimentService(nil))

	body := `{"description":"test"}`
	req := httptest.NewRequest("POST", "/datasets?project_id=proj-123", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	handler.CreateDataset(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestListDatasetsHandler_MissingProjectID(t *testing.T) {
	handler := api.NewDatasetHandler(services.NewDatasetService(nil), services.NewExperimentService(nil))

	req := httptest.NewRequest("GET", "/datasets", nil)
	w := httptest.NewRecorder()
	handler.ListDatasets(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestCreateDatasetRunHandler_MissingName(t *testing.T) {
	handler := api.NewDatasetHandler(services.NewDatasetService(nil), services.NewExperimentService(nil))

	body := `{}`
	req := httptest.NewRequest("POST", "/datasets/ds-123/runs", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	handler.CreateDatasetRun(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestCreateDatasetRunItemHandler_MissingDatasetItemID(t *testing.T) {
	handler := api.NewDatasetHandler(services.NewDatasetService(nil), services.NewExperimentService(nil))

	body := `{"observation_id":"obs-123"}`
	req := httptest.NewRequest("POST", "/datasets/ds-123/runs/run-123/items", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	handler.CreateDatasetRunItem(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestAPIKeyMiddleware_MissingHeader(t *testing.T) {
	// Test that APIKeyMiddleware rejects requests without x-api-key
	middleware := auth.APIKeyMiddleware(nil)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestAPIKeyMiddleware_EmptyHeader(t *testing.T) {
	middleware := auth.APIKeyMiddleware(nil)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("x-api-key", "")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}
