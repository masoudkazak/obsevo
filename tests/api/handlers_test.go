package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/obsevo/obsevo/internal/api"
	"github.com/obsevo/obsevo/internal/auth"
	"github.com/obsevo/obsevo/internal/services"
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

func TestSecurityHeaders(t *testing.T) {
	handler := api.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	expectedHeaders := map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"Referrer-Policy":           "no-referrer",
		"X-XSS-Protection":          "1; mode=block",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		"Content-Security-Policy":   "default-src 'self'",
	}

	for header, expected := range expectedHeaders {
		got := w.Header().Get(header)
		if !strings.Contains(got, expected) {
			t.Errorf("header %q: expected to contain %q, got %q", header, expected, got)
		}
	}
}

func TestLimitBody(t *testing.T) {
	// Handler that tries to read the body
	handler := api.LimitBody(1024)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 2048)
		n, err := r.Body.Read(buf)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Write(buf[:n])
	}))

	// Send a body larger than the limit
	body := strings.Repeat("x", 2048)
	req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Error("expected error for body exceeding limit, got 200")
	}
}

func TestLimitBody_UnderLimit(t *testing.T) {
	handler := api.LimitBody(1024)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		w.Write(buf[:n])
	}))

	body := strings.Repeat("x", 512)
	req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for body under limit, got %d", w.Code)
	}
}

func TestRequestLogger(t *testing.T) {
	handler := api.RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Should set X-Request-Id header on response
	if w.Header().Get("X-Request-Id") == "" {
		t.Error("expected X-Request-Id header to be set")
	}
}

func TestRequestLogger_PreservesClientID(t *testing.T) {
	handler := api.RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-Id", "my-custom-id")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("X-Request-Id"); got != "my-custom-id" {
		t.Errorf("expected X-Request-Id 'my-custom-id', got %q", got)
	}
}

func TestSetupTestRouter(t *testing.T) {
	r := setupTestRouter()
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}
