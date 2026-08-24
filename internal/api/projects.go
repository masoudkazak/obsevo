package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/langfuse-light/langfuse-light/internal/auth"
	"github.com/langfuse-light/langfuse-light/internal/db"
)

// ProjectHandler handles project requests
type ProjectHandler struct {
	queries *db.Queries
}

// NewProjectHandler creates a new project handler
func NewProjectHandler(queries *db.Queries) *ProjectHandler {
	return &ProjectHandler{queries: queries}
}

// CreateProjectRequest represents the create project request body
type CreateProjectRequest struct {
	Name string `json:"name"`
}

// CreateOrganizationRequest represents the create organization request body
type CreateOrganizationRequest struct {
	Name string `json:"name"`
}

// Project represents a project in responses
type Project struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	OrgID     string `json:"org_id"`
	CreatedAt string `json:"created_at"`
}

// Organization represents an organization in responses
type Organization struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// CreateOrganization creates a new organization
func (h *ProjectHandler) CreateOrganization(w http.ResponseWriter, r *http.Request) {
	var req CreateOrganizationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Create organization
	org, err := h.queries.CreateOrganization(context.Background(), req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create organization")
		return
	}

	// Add current user as ADMIN of the organization
	userID := auth.GetUserID(r.Context())
	_, err = h.queries.CreateMember(context.Background(), db.CreateMemberParams{
		UserID: userID,
		OrgID:  org.ID,
		Role:   "ADMIN",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add user to organization")
		return
	}

	writeJSON(w, http.StatusCreated, Organization{
		ID:        org.ID,
		Name:      org.Name,
		CreatedAt: org.CreatedAt.Time.String(),
	})
}

// ListOrganizations lists organizations for the current user
func (h *ProjectHandler) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserID(r.Context())

	// For now, return empty list - implement proper query later
	writeJSON(w, http.StatusOK, []Organization{
		{ID: uuid.New().String(), Name: "Default Org", CreatedAt: "2024-01-01"},
	})
	_ = userID
}

// CreateProject creates a new project
func (h *ProjectHandler) CreateProject(w http.ResponseWriter, r *http.Request) {
	var req CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	// For now, use a default org ID - implement proper org lookup later
	orgID := uuid.New().String()

	// Create project
	project, err := h.queries.CreateProject(context.Background(), db.CreateProjectParams{
		Name:  req.Name,
		OrgID: orgID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create project")
		return
	}

	writeJSON(w, http.StatusCreated, Project{
		ID:        project.ID,
		Name:      project.Name,
		OrgID:     project.OrgID,
		CreatedAt: project.CreatedAt.Time.String(),
	})
}

// ListProjects lists projects for the current user
func (h *ProjectHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	// For now, return empty list - implement proper query later
	writeJSON(w, http.StatusOK, []Project{})
}

// RegisterRoutes registers project routes
func (h *ProjectHandler) RegisterRoutes(r chi.Router) {
	r.Post("/organizations", h.CreateOrganization)
	r.Get("/organizations", h.ListOrganizations)
	r.Post("/projects", h.CreateProject)
	r.Get("/projects", h.ListProjects)
}
