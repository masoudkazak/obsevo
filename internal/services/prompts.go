package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/obsevo/obsevo/internal/db"
)

// Prompt types, matching Langfuse.
const (
	PromptTypeText = "text"
	PromptTypeChat = "chat"
)

// LabelProduction is the label a prompt is served under when no label is asked
// for, matching Langfuse's default.
const LabelProduction = "production"

// LabelLatest always points at the highest version of a prompt.
const LabelLatest = "latest"

// PromptService handles prompt and template business logic.
type PromptService struct {
	queries *db.Queries
}

// NewPromptService creates a new prompt service.
func NewPromptService(queries *db.Queries) *PromptService {
	return &PromptService{queries: queries}
}

// ChatMessage is one message of a chat prompt.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CreatePromptRequest is the request body for creating a prompt version.
//
// Prompt accepts either shape Langfuse supports: a JSON string for a text
// prompt, or an array of {role, content} objects for a chat prompt. A bare
// JSON string is what the pre-existing API sent, so older clients are
// unaffected.
type CreatePromptRequest struct {
	Name          string          `json:"name"`
	Prompt        json.RawMessage `json:"prompt"`
	Config        json.RawMessage `json:"config"`
	Type          string          `json:"type"`
	Labels        []string        `json:"labels"`
	Tags          []string        `json:"tags"`
	CommitMessage string          `json:"commit_message"`
	CreatedBy     string          `json:"created_by"`

	// IsActive is the pre-label way of saying "serve this version". When true
	// and no labels are given, the version receives the production label.
	IsActive bool `json:"is_active"`
}

// CreatePrompt creates version 1 of a new prompt.
func (s *PromptService) CreatePrompt(ctx context.Context, projectID string, req CreatePromptRequest) (db.Prompt, error) {
	if req.Name == "" {
		return db.Prompt{}, fmt.Errorf("name is required")
	}

	max, err := s.queries.GetMaxPromptVersion(ctx, db.GetMaxPromptVersionParams{
		ProjectID: projectID,
		Name:      req.Name,
	})
	if err != nil {
		return db.Prompt{}, fmt.Errorf("checking existing prompt versions: %w", err)
	}
	if max > 0 {
		return db.Prompt{}, fmt.Errorf("prompt with name %q already exists in project", req.Name)
	}

	return s.createVersion(ctx, projectID, req.Name, 1, req)
}

// CreatePromptVersionRequest is the request body for adding a prompt version.
type CreatePromptVersionRequest struct {
	Prompt        json.RawMessage `json:"prompt"`
	Config        json.RawMessage `json:"config"`
	Type          string          `json:"type"`
	Labels        []string        `json:"labels"`
	Tags          []string        `json:"tags"`
	CommitMessage string          `json:"commit_message"`
	CreatedBy     string          `json:"created_by"`
	IsActive      bool            `json:"is_active"`
}

// CreatePromptVersion adds a new version of an existing prompt.
func (s *PromptService) CreatePromptVersion(ctx context.Context, projectID, name string, req CreatePromptVersionRequest) (db.Prompt, error) {
	max, err := s.queries.GetMaxPromptVersion(ctx, db.GetMaxPromptVersionParams{
		ProjectID: projectID,
		Name:      name,
	})
	if err != nil {
		return db.Prompt{}, fmt.Errorf("looking up prompt versions: %w", err)
	}
	if max == 0 {
		return db.Prompt{}, fmt.Errorf("prompt %q not found in project", name)
	}

	return s.createVersion(ctx, projectID, name, max+1, CreatePromptRequest{
		Name:          name,
		Prompt:        req.Prompt,
		Config:        req.Config,
		Type:          req.Type,
		Labels:        req.Labels,
		Tags:          req.Tags,
		CommitMessage: req.CommitMessage,
		CreatedBy:     req.CreatedBy,
		IsActive:      req.IsActive,
	})
}

// createVersion writes one prompt version and applies its labels.
func (s *PromptService) createVersion(ctx context.Context, projectID, name string, version int32, req CreatePromptRequest) (db.Prompt, error) {
	body, promptType, err := normalizePromptBody(req.Prompt, req.Type)
	if err != nil {
		return db.Prompt{}, err
	}

	config := req.Config
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	}

	labels := normalizeLabels(req.Labels)
	if len(labels) == 0 && req.IsActive {
		labels = []string{LabelProduction}
	}

	prompt, err := s.queries.CreatePromptVersion(ctx, db.CreatePromptVersionParams{
		ProjectID:     projectID,
		Name:          name,
		Version:       version,
		Prompt:        body,
		Config:        config,
		IsActive:      containsLabel(labels, LabelProduction),
		Type:          promptType,
		Labels:        labels,
		Tags:          normalizeLabels(req.Tags),
		CommitMessage: req.CommitMessage,
		CreatedBy:     req.CreatedBy,
	})
	if err != nil {
		return db.Prompt{}, fmt.Errorf("creating prompt version: %w", err)
	}

	// A label points at exactly one version, so strip it from the others.
	for _, label := range labels {
		if err := s.moveLabel(ctx, projectID, name, label, version); err != nil {
			return db.Prompt{}, err
		}
	}
	if err := s.syncActive(ctx, projectID, name); err != nil {
		return db.Prompt{}, err
	}

	prompt.Labels = labels
	return prompt, nil
}

// SetPromptLabelsRequest assigns labels to one version of a prompt.
type SetPromptLabelsRequest struct {
	Version int32    `json:"version"`
	Labels  []string `json:"labels"`
}

// SetPromptLabels points the given labels at a specific version. This is how a
// rollback is performed: move `production` back to an earlier version.
func (s *PromptService) SetPromptLabels(ctx context.Context, projectID, name string, req SetPromptLabelsRequest) error {
	if req.Version <= 0 {
		return fmt.Errorf("version must be positive")
	}
	labels := normalizeLabels(req.Labels)
	if len(labels) == 0 {
		return fmt.Errorf("at least one label is required")
	}

	if _, err := s.queries.GetPromptByNameAndVersion(ctx, db.GetPromptByNameAndVersionParams{
		ProjectID: projectID,
		Name:      name,
		Version:   req.Version,
	}); err != nil {
		return fmt.Errorf("prompt version not found: %w", err)
	}

	for _, label := range labels {
		if err := s.moveLabel(ctx, projectID, name, label, req.Version); err != nil {
			return err
		}
	}

	return s.syncActive(ctx, projectID, name)
}

// SetPromptActiveRequest is the pre-label way of selecting a served version.
type SetPromptActiveRequest struct {
	Version int32 `json:"version"`
}

// SetPromptActive points the production label at a version. It is the original
// endpoint's behaviour expressed in terms of labels.
func (s *PromptService) SetPromptActive(ctx context.Context, projectID, name string, version int32) error {
	return s.SetPromptLabels(ctx, projectID, name, SetPromptLabelsRequest{
		Version: version,
		Labels:  []string{LabelProduction},
	})
}

// moveLabel makes a label point at exactly one version.
func (s *PromptService) moveLabel(ctx context.Context, projectID, name, label string, version int32) error {
	err := s.queries.RemoveLabelFromPrompts(ctx, db.RemoveLabelFromPromptsParams{
		ProjectID: projectID,
		Name:      name,
		Label:     label,
	})
	if err != nil {
		return fmt.Errorf("clearing label %q: %w", label, err)
	}

	err = s.queries.AddLabelToPromptVersion(ctx, db.AddLabelToPromptVersionParams{
		ProjectID: projectID,
		Name:      name,
		Version:   version,
		Label:     label,
	})
	if err != nil {
		return fmt.Errorf("applying label %q: %w", label, err)
	}
	return nil
}

// syncActive keeps is_active aligned with the production label.
func (s *PromptService) syncActive(ctx context.Context, projectID, name string) error {
	err := s.queries.SyncPromptActiveFromLabels(ctx, db.SyncPromptActiveFromLabelsParams{
		ProjectID: projectID,
		Name:      name,
	})
	if err != nil {
		return fmt.Errorf("syncing prompt active flag: %w", err)
	}
	return nil
}

// ListPromptsResponse is the response for listing prompts.
type ListPromptsResponse struct {
	Prompts []db.Prompt             `json:"prompts"`
	Names   []db.ListPromptNamesRow `json:"names"`
}

// ListPrompts lists all prompt versions for a project, plus a per-name summary.
func (s *PromptService) ListPrompts(ctx context.Context, projectID string) (ListPromptsResponse, error) {
	prompts, err := s.queries.GetPromptsByProjectID(ctx, projectID)
	if err != nil {
		return ListPromptsResponse{}, fmt.Errorf("listing prompts: %w", err)
	}

	names, err := s.queries.ListPromptNames(ctx, projectID)
	if err != nil {
		return ListPromptsResponse{}, fmt.Errorf("listing prompt names: %w", err)
	}

	return ListPromptsResponse{Prompts: prompts, Names: names}, nil
}

// GetPrompt resolves a prompt by label or version, matching Langfuse's
// resolution order: an explicit version wins, then a label, then production,
// then the highest version.
func (s *PromptService) GetPrompt(ctx context.Context, projectID, name string, version int32, label string) (db.Prompt, error) {
	if version > 0 {
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

	if label == LabelLatest {
		return s.getLatest(ctx, projectID, name)
	}

	if label == "" {
		label = LabelProduction
	}

	prompt, err := s.queries.GetPromptByLabel(ctx, db.GetPromptByLabelParams{
		ProjectID: projectID,
		Name:      name,
		Label:     label,
	})
	if err == nil {
		return prompt, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.Prompt{}, fmt.Errorf("getting prompt by label: %w", err)
	}

	// Nothing carries the requested label. Falling back to the newest version
	// keeps a prompt usable immediately after it is created, which is what the
	// pre-label API did.
	if label == LabelProduction {
		return s.getLatest(ctx, projectID, name)
	}
	return db.Prompt{}, fmt.Errorf("no prompt version carries label %q", label)
}

func (s *PromptService) getLatest(ctx context.Context, projectID, name string) (db.Prompt, error) {
	prompt, err := s.queries.GetLatestPrompt(ctx, db.GetLatestPromptParams{
		ProjectID: projectID,
		Name:      name,
	})
	if err != nil {
		return db.Prompt{}, fmt.Errorf("getting prompt: %w", err)
	}
	return prompt, nil
}

// GetPromptByName returns the production version of a prompt.
func (s *PromptService) GetPromptByName(ctx context.Context, projectID, name string) (db.Prompt, error) {
	return s.GetPrompt(ctx, projectID, name, 0, LabelProduction)
}

// GetPromptByVersion returns a specific version of a prompt.
func (s *PromptService) GetPromptByVersion(ctx context.Context, projectID, name string, version int32) (db.Prompt, error) {
	return s.GetPrompt(ctx, projectID, name, version, "")
}

// GetPromptVersions returns all versions of a prompt, newest first.
func (s *PromptService) GetPromptVersions(ctx context.Context, projectID, name string) ([]db.Prompt, error) {
	versions, err := s.queries.GetPromptVersions(ctx, db.GetPromptVersionsParams{
		ProjectID: projectID,
		Name:      name,
	})
	if err != nil {
		return nil, fmt.Errorf("listing prompt versions: %w", err)
	}
	return versions, nil
}

// DeletePromptVersion removes one version of a prompt.
func (s *PromptService) DeletePromptVersion(ctx context.Context, projectID, name string, version int32) error {
	err := s.queries.DeletePromptVersion(ctx, db.DeletePromptVersionParams{
		ProjectID: projectID,
		Name:      name,
		Version:   version,
	})
	if err != nil {
		return fmt.Errorf("deleting prompt version: %w", err)
	}
	return nil
}

// DeletePrompt removes every version of a prompt.
func (s *PromptService) DeletePrompt(ctx context.Context, projectID, name string) error {
	err := s.queries.DeletePromptByName(ctx, db.DeletePromptByNameParams{
		ProjectID: projectID,
		Name:      name,
	})
	if err != nil {
		return fmt.Errorf("deleting prompt: %w", err)
	}
	return nil
}

// --- templates ------------------------------------------------------------

// variablePattern matches the {{name}} placeholders used in prompt templates.
var variablePattern = regexp.MustCompile(`\{\{\s*(\w+)\s*\}\}`)

// CompileTemplate substitutes {{variable}} placeholders in a template.
func (s *PromptService) CompileTemplate(promptStr string, variables map[string]string) (string, error) {
	matches := variablePattern.FindAllStringSubmatch(promptStr, -1)
	if len(matches) == 0 {
		return promptStr, nil
	}

	var undefined []string
	seen := map[string]bool{}
	for _, match := range matches {
		name := match[1]
		if _, ok := variables[name]; !ok && !seen[name] {
			seen[name] = true
			undefined = append(undefined, name)
		}
	}
	if len(undefined) > 0 {
		return "", fmt.Errorf("undefined template variables: %s", strings.Join(undefined, ", "))
	}

	return variablePattern.ReplaceAllStringFunc(promptStr, func(match string) string {
		name := variablePattern.FindStringSubmatch(match)[1]
		return variables[name]
	}), nil
}

// ExtractTemplateVariables lists the distinct {{variable}} names in a template,
// in first-appearance order.
func (s *PromptService) ExtractTemplateVariables(promptStr string) []string {
	matches := variablePattern.FindAllStringSubmatch(promptStr, -1)

	seen := make(map[string]bool, len(matches))
	variables := make([]string, 0, len(matches))
	for _, match := range matches {
		name := match[1]
		if !seen[name] {
			seen[name] = true
			variables = append(variables, name)
		}
	}
	return variables
}

// PromptWithCompiledTemplate is a prompt plus its decoded template details.
type PromptWithCompiledTemplate struct {
	db.Prompt
	// PromptText is the decoded template: the text itself for a text prompt,
	// or the concatenated message contents for a chat prompt.
	PromptText       string        `json:"prompt_text"`
	ChatMessages     []ChatMessage `json:"chat_messages,omitempty"`
	CompiledTemplate string        `json:"compiled_template,omitempty"`
	Variables        []string      `json:"variables"`
}

// Describe decodes a stored prompt into its template form.
func (s *PromptService) Describe(prompt db.Prompt) PromptWithCompiledTemplate {
	out := PromptWithCompiledTemplate{Prompt: prompt}

	if prompt.Type == PromptTypeChat {
		var messages []ChatMessage
		if err := json.Unmarshal(prompt.Prompt, &messages); err == nil {
			out.ChatMessages = messages
			parts := make([]string, 0, len(messages))
			for _, m := range messages {
				parts = append(parts, m.Content)
			}
			out.PromptText = strings.Join(parts, "\n")
		}
	} else {
		var text string
		if err := json.Unmarshal(prompt.Prompt, &text); err == nil {
			out.PromptText = text
		} else {
			out.PromptText = string(prompt.Prompt)
		}
	}

	out.Variables = s.ExtractTemplateVariables(out.PromptText)
	if out.Variables == nil {
		out.Variables = []string{}
	}
	return out
}

// GetPromptByNameWithTemplate returns the production prompt with template info.
func (s *PromptService) GetPromptByNameWithTemplate(ctx context.Context, projectID, name string) (PromptWithCompiledTemplate, error) {
	prompt, err := s.GetPromptByName(ctx, projectID, name)
	if err != nil {
		return PromptWithCompiledTemplate{}, err
	}
	return s.Describe(prompt), nil
}

// --- encoding helpers -----------------------------------------------------

// normalizePromptBody validates the prompt body and infers its type.
//
// The prompt column is JSONB. A text prompt is stored as a JSON string and a
// chat prompt as an array of messages, matching Langfuse. Callers that send a
// bare, unquoted template string are accommodated by encoding it as a JSON
// string, so the original plain-text API keeps working.
func normalizePromptBody(raw json.RawMessage, declaredType string) (json.RawMessage, string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, "", fmt.Errorf("prompt is required")
	}

	declaredType = strings.ToLower(strings.TrimSpace(declaredType))

	// A JSON string is a text prompt.
	var asText string
	if err := json.Unmarshal(raw, &asText); err == nil {
		if declaredType == PromptTypeChat {
			return nil, "", fmt.Errorf("prompt type is chat but the body is a string")
		}
		if asText == "" {
			return nil, "", fmt.Errorf("prompt is required")
		}
		encoded, err := json.Marshal(asText)
		if err != nil {
			return nil, "", fmt.Errorf("encoding prompt: %w", err)
		}
		return encoded, PromptTypeText, nil
	}

	// An array of {role, content} is a chat prompt.
	var messages []ChatMessage
	if err := json.Unmarshal(raw, &messages); err == nil && len(messages) > 0 {
		for i, m := range messages {
			if m.Role == "" {
				return nil, "", fmt.Errorf("chat message %d is missing a role", i)
			}
		}
		if declaredType == PromptTypeText {
			return nil, "", fmt.Errorf("prompt type is text but the body is a message list")
		}
		encoded, err := json.Marshal(messages)
		if err != nil {
			return nil, "", fmt.Errorf("encoding chat prompt: %w", err)
		}
		return encoded, PromptTypeChat, nil
	}

	// Anything else that is valid JSON is stored as-is and treated as text.
	if json.Valid(raw) {
		return raw, PromptTypeText, nil
	}

	// Not JSON at all: treat the payload as a literal template string.
	encoded, err := json.Marshal(trimmed)
	if err != nil {
		return nil, "", fmt.Errorf("encoding prompt: %w", err)
	}
	return encoded, PromptTypeText, nil
}

// normalizeLabels trims, lowercases and de-duplicates a label list.
func normalizeLabels(labels []string) []string {
	if len(labels) == 0 {
		return []string{}
	}

	seen := make(map[string]bool, len(labels))
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		label = strings.ToLower(strings.TrimSpace(label))
		if label == "" || seen[label] {
			continue
		}
		seen[label] = true
		out = append(out, label)
	}
	return out
}

func containsLabel(labels []string, want string) bool {
	for _, label := range labels {
		if label == want {
			return true
		}
	}
	return false
}
