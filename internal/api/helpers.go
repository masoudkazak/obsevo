package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/langfuse-light/langfuse-light/internal/auth"
	"github.com/langfuse-light/langfuse-light/internal/services"
)

// requireProject returns the authorized project for the request. The project was
// already validated against the caller's membership by auth.ProjectMiddleware
// (JWT routes) or auth.APIKeyMiddleware (SDK routes), so an empty value here
// means the client simply did not name one.
func requireProject(w http.ResponseWriter, r *http.Request) (string, bool) {
	projectID := auth.GetProjectID(r.Context())
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return "", false
	}
	return projectID, true
}

// queryInt32 reads an integer query parameter, returning def when absent or invalid.
func queryInt32(r *http.Request, key string, def int32) int32 {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return def
	}
	v, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return def
	}
	return int32(v)
}

// queryCSV reads a comma-separated query parameter into a slice, dropping blanks.
func queryCSV(r *http.Request, key string) []string {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// writeServiceError maps a service-layer error onto an HTTP status.
func writeServiceError(w http.ResponseWriter, err error, notFoundMessage string) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, notFoundMessage)
	case errors.Is(err, services.ErrCrossProject):
		writeError(w, http.StatusForbidden, "entity belongs to a different project")
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}
