package services

import (
	"context"
	"fmt"

	"github.com/langfuse-light/langfuse-light/internal/db"
)

// ProjectService handles project and organization business logic.
type ProjectService struct {
	queries *db.Queries
}

// NewProjectService creates a new project service.
func NewProjectService(queries *db.Queries) *ProjectService {
	return &ProjectService{queries: queries}
}

// CreateOrganizationRequest is the request body for creating an organization.
type CreateOrganizationRequest struct {
	Name string `json:"name"`
}

// CreateOrganization creates a new organization and adds the user as ADMIN.
func (s *ProjectService) CreateOrganization(ctx context.Context, userID string, req CreateOrganizationRequest) (db.Organization, error) {
	if req.Name == "" {
		return db.Organization{}, fmt.Errorf("name is required")
	}

	org, err := s.queries.CreateOrganization(ctx, req.Name)
	if err != nil {
		return db.Organization{}, fmt.Errorf("creating organization: %w", err)
	}

	_, err = s.queries.CreateMember(ctx, db.CreateMemberParams{
		UserID: userID,
		OrgID:  org.ID,
		Role:   "ADMIN",
	})
	if err != nil {
		return db.Organization{}, fmt.Errorf("adding user to organization: %w", err)
	}

	return org, nil
}

// ListOrganizations returns all organizations for a user.
func (s *ProjectService) ListOrganizations(ctx context.Context, userID string) ([]db.Organization, error) {
	orgs, err := s.queries.GetOrganizationsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing organizations: %w", err)
	}
	return orgs, nil
}

// CreateProjectRequest is the request body for creating a project.
type CreateProjectRequest struct {
	Name string `json:"name"`
}

// CreateProject creates a new project under the user's first organization.
func (s *ProjectService) CreateProject(ctx context.Context, userID string, req CreateProjectRequest) (db.Project, error) {
	if req.Name == "" {
		return db.Project{}, fmt.Errorf("name is required")
	}

	orgs, err := s.queries.GetOrganizationsByUserID(ctx, userID)
	if err != nil || len(orgs) == 0 {
		return db.Project{}, fmt.Errorf("user must belong to an organization first")
	}

	project, err := s.queries.CreateProject(ctx, db.CreateProjectParams{
		Name:  req.Name,
		OrgID: orgs[0].ID,
	})
	if err != nil {
		return db.Project{}, fmt.Errorf("creating project: %w", err)
	}

	return project, nil
}

// ListProjects returns all projects across the user's organizations.
func (s *ProjectService) ListProjects(ctx context.Context, userID string) ([]db.Project, error) {
	orgs, err := s.queries.GetOrganizationsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing organizations: %w", err)
	}

	var allProjects []db.Project
	for _, org := range orgs {
		projects, err := s.queries.GetProjectsByOrgID(ctx, org.ID)
		if err != nil {
			continue
		}
		allProjects = append(allProjects, projects...)
	}

	if allProjects == nil {
		allProjects = []db.Project{}
	}

	return allProjects, nil
}

// GetProject returns a project by ID.
func (s *ProjectService) GetProject(ctx context.Context, id string) (db.Project, error) {
	project, err := s.queries.GetProjectByID(ctx, id)
	if err != nil {
		return db.Project{}, fmt.Errorf("getting project: %w", err)
	}
	return project, nil
}
