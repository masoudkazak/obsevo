package services

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/langfuse-light/langfuse-light/internal/db"
)

// PromptService handles prompt and template business logic.
type PromptService struct {
	queries *db.Queries
}

// NewPromptService creates a new prompt service.
func NewPromptService(queries *db.Queries) *PromptService {
	return &PromptService{queries: queries}
}

// CreatePromptRequest is the request body for creating a prompt.
type CreatePromptRequest struct {
	Name     string          `json:"name"`
	Prompt   string          `json:"prompt"`
	Config   json.RawMessage `json:"config"`
	IsActive bool            `json:"is_active"`
}

// CreatePrompt creates a new prompt version or updates an existing prompt's active status.
func (s *PromptService) CreatePrompt(ctx context.Context, projectID string, req CreatePromptRequest) (db.Prompt, error) {
	existing, err := s.queries.GetPromptsByProjectID(ctx, projectID)
	if err == nil {
		for _, p := range existing {
			if p.Name == req.Name {
				return db.Prompt{}, fmt.Errorf("prompt with name %q already exists in project", req.Name)
			}
		}
	}

	version := int32(1)
	config := req.Config
	if config == nil {
		config = json.RawMessage(`{}`)
	}

	prompt, err := s.queries.CreatePrompt(ctx, db.CreatePromptParams{
		ProjectID: projectID,
		Name:      req.Name,
		Version:   version,
		Prompt:    []byte(req.Prompt),
		Config:    config,
		IsActive:  req.IsActive,
	})
	if err != nil {
		return db.Prompt{}, fmt.Errorf("creating prompt: %w", err)
	}

	return prompt, nil
}

// CreatePromptVersionRequest is the request body for creating a new prompt version.
type CreatePromptVersionRequest struct {
	Prompt   string          `json:"prompt"`
	Config   json.RawMessage `json:"config"`
	IsActive bool            `json:"is_active"`
}

// CreatePromptVersion creates a new version of an existing prompt.
func (s *PromptService) CreatePromptVersion(ctx context.Context, projectID, name string, req CreatePromptVersionRequest) (db.Prompt, error) {
	existing, err := s.queries.GetPromptsByProjectID(ctx, projectID)
	if err != nil {
		return db.Prompt{}, fmt.Errorf("listing prompts: %w", err)
	}

	maxVersion := int32(0)
	for _, p := range existing {
		if p.Name == name && p.Version > maxVersion {
			maxVersion = p.Version
		}
	}

	if maxVersion == 0 {
		return db.Prompt{}, fmt.Errorf("prompt %q not found in project", name)
	}

	newVersion := maxVersion + 1
	config := req.Config
	if config == nil {
		config = json.RawMessage(`{}`)
	}

	prompt, err := s.queries.CreatePrompt(ctx, db.CreatePromptParams{
		ProjectID: projectID,
		Name:      name,
		Version:   newVersion,
		Prompt:    []byte(req.Prompt),
		Config:    config,
		IsActive:  req.IsActive,
	})
	if err != nil {
		return db.Prompt{}, fmt.Errorf("creating prompt version: %w", err)
	}

	return prompt, nil
}

// ListPromptsResponse is the response for listing prompts.
type ListPromptsResponse struct {
	Prompts []db.Prompt `json:"prompts"`
}

// ListPrompts lists all prompts for a project.
func (s *PromptService) ListPrompts(ctx context.Context, projectID string) (ListPromptsResponse, error) {
	prompts, err := s.queries.GetPromptsByProjectID(ctx, projectID)
	if err != nil {
		return ListPromptsResponse{}, fmt.Errorf("listing prompts: %w", err)
	}

	return ListPromptsResponse{Prompts: prompts}, nil
}

// GetPromptByName returns the active version of a prompt by name.
func (s *PromptService) GetPromptByName(ctx context.Context, projectID, name string) (db.Prompt, error) {
	prompt, err := s.queries.GetActivePromptByName(ctx, db.GetActivePromptByNameParams{
		ProjectID: projectID,
		Name:      name,
	})
	if err != nil {
		return db.Prompt{}, fmt.Errorf("getting prompt: %w", err)
	}

	return prompt, nil
}

// GetPromptByVersion returns a specific version of a prompt.
func (s *PromptService) GetPromptByVersion(ctx context.Context, projectID, name string, version int32) (db.Prompt, error) {
	prompt, err := s.queries.GetPromptByNameAndVersion(ctx, db.GetPromptByNameAndVersionParams{
		ProjectID: projectID,
		Name:      name,
		Version:   version,
	})
	if err != nil {
		return db.Prompt{}, fmt.Errorf("getting prompt version: %w", err)
	}

	return prompt, nil
}

// GetPromptVersions returns all versions of a prompt by name.
func (s *PromptService) GetPromptVersions(ctx context.Context, projectID, name string) ([]db.Prompt, error) {
	all, err := s.queries.GetPromptsByProjectID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("listing prompts: %w", err)
	}

	var versions []db.Prompt
	for _, p := range all {
		if p.Name == name {
			versions = append(versions, p)
		}
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Version > versions[j].Version
	})

	return versions, nil
}

// SetPromptActiveRequest is the request body for setting a prompt version as active.
type SetPromptActiveRequest struct {
	Version int32 `json:"version"`
}

// SetPromptActive sets a specific version of a prompt as active and deactivates others.
func (s *PromptService) SetPromptActive(ctx context.Context, projectID, name string, version int32) error {
	_, err := s.queries.GetPromptByNameAndVersion(ctx, db.GetPromptByNameAndVersionParams{
		ProjectID: projectID,
		Name:      name,
		Version:   version,
	})
	if err != nil {
		return fmt.Errorf("prompt version not found: %w", err)
	}

	err = s.queries.UpdatePromptActive(ctx, db.UpdatePromptActiveParams{
		ProjectID: projectID,
		Name:      name,
		IsActive:  false,
	})
	if err != nil {
		return fmt.Errorf("deactivating prompts: %w", err)
	}

	err = s.queries.UpdatePromptActiveByVersion(ctx, db.UpdatePromptActiveByVersionParams{
		ProjectID: projectID,
		Name:      name,
		IsActive:  true,
		Version:   version,
	})
	if err != nil {
		return fmt.Errorf("activating prompt version: %w", err)
	}

	return nil
}

// CompileTemplate compiles a prompt template by substituting variables.
// Variables are denoted by {{variable_name}} in the template.
func (s *PromptService) CompileTemplate(promptStr string, variables map[string]string) (string, error) {
	varPattern := regexp.MustCompile(`\{\{(\w+)\}\}`)

	matches := varPattern.FindAllStringSubmatch(promptStr, -1)
	if len(matches) == 0 {
		return promptStr, nil
	}

	var undefinedVars []string
	for _, match := range matches {
		varName := match[1]
		if _, ok := variables[varName]; !ok {
			undefinedVars = append(undefinedVars, varName)
		}
	}

	if len(undefinedVars) > 0 {
		return "", fmt.Errorf("undefined template variables: %s", strings.Join(undefinedVars, ", "))
	}

	result := varPattern.ReplaceAllStringFunc(promptStr, func(match string) string {
		varName := match[2 : len(match)-2]
		return variables[varName]
	})

	return result, nil
}

// ExtractTemplateVariables extracts variable names from a prompt template.
func (s *PromptService) ExtractTemplateVariables(promptStr string) []string {
	varPattern := regexp.MustCompile(`\{\{(\w+)\}\}`)
	matches := varPattern.FindAllStringSubmatch(promptStr, -1)

	seen := make(map[string]bool)
	var variables []string
	for _, match := range matches {
		varName := match[1]
		if !seen[varName] {
			seen[varName] = true
			variables = append(variables, varName)
		}
	}

	return variables
}

// PromptWithCompiledTemplate represents a prompt with its compiled template.
type PromptWithCompiledTemplate struct {
	db.Prompt
	CompiledTemplate string   `json:"compiled_template,omitempty"`
	Variables        []string `json:"variables,omitempty"`
}

// GetPromptByNameWithTemplate returns the active prompt and its template info.
func (s *PromptService) GetPromptByNameWithTemplate(ctx context.Context, projectID, name string) (PromptWithCompiledTemplate, error) {
	prompt, err := s.GetPromptByName(ctx, projectID, name)
	if err != nil {
		return PromptWithCompiledTemplate{}, err
	}

	promptStr := string(prompt.Prompt)
	variables := s.ExtractTemplateVariables(promptStr)

	return PromptWithCompiledTemplate{
		Prompt:    prompt,
		Variables: variables,
	}, nil
}
