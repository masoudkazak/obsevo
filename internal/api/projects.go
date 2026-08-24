package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

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

	orgs, err := h.queries.GetOrganizationsByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list organizations: "+err.Error())
		return
	}

	result := make([]Organization, len(orgs))
	for i, o := range orgs {
		result[i] = Organization{
			ID:        o.ID,
			Name:      o.Name,
			CreatedAt: o.CreatedAt.Time.String(),
		}
	}

	writeJSON(w, http.StatusOK, result)
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

	userID := auth.GetUserID(r.Context())
	orgs, err := h.queries.GetOrganizationsByUserID(r.Context(), userID)
	if err != nil || len(orgs) == 0 {
		writeError(w, http.StatusBadRequest, "user must belong to an organization first")
		return
	}

	orgID := orgs[0].ID

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
	userID := auth.GetUserID(r.Context())

	orgs, err := h.queries.GetOrganizationsByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list organizations: "+err.Error())
		return
	}

	var allProjects []Project
	for _, org := range orgs {
		projects, err := h.queries.GetProjectsByOrgID(r.Context(), org.ID)
		if err != nil {
			continue
		}
		for _, p := range projects {
			allProjects = append(allProjects, Project{
				ID:        p.ID,
				Name:      p.Name,
				OrgID:     p.OrgID,
				CreatedAt: p.CreatedAt.Time.String(),
			})
		}
	}

	if allProjects == nil {
		allProjects = []Project{}
	}

	writeJSON(w, http.StatusOK, allProjects)
}

// RegisterRoutes registers project routes
func (h *ProjectHandler) RegisterRoutes(r chi.Router) {
	r.Post("/organizations", h.CreateOrganization)
	r.Get("/organizations", h.ListOrganizations)
	r.Post("/projects", h.CreateProject)
	r.Get("/projects", h.ListProjects)
}
