package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/langfuse-light/langfuse-light/internal/auth"
	"github.com/langfuse-light/langfuse-light/internal/db"
)

// SettingsHandler handles settings and management HTTP requests.
type SettingsHandler struct {
	queries *db.Queries
}

// NewSettingsHandler creates a new settings handler.
func NewSettingsHandler(queries *db.Queries) *SettingsHandler {
	return &SettingsHandler{queries: queries}
}

// APIKey represents an API key in responses.
type APIKey struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Key       string `json:"key"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// CreateAPIKeyRequest is the request body for creating an API key.
type CreateAPIKeyRequest struct {
	Name string `json:"name"`
}

// Member represents a member in responses.
type Member struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id"`
	OrgID    string `json:"org_id"`
	Role     string `json:"role"`
	Email    string `json:"email"`
	UserName string `json:"user_name"`
}

// UpdateMemberRoleRequest is the request body for updating a member's role.
type UpdateMemberRoleRequest struct {
	Role string `json:"role"`
}

// UpdateProfileRequest is the request body for updating user profile.
type UpdateProfileRequest struct {
	Name string `json:"name"`
}

// UserWithProfile represents a user profile in responses.
type UserWithProfile struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// ListAPIKeys handles GET /api/api-keys.
func (h *SettingsHandler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	keys, err := h.queries.GetAPIKeysByProjectID(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list API keys: "+err.Error())
		return
	}

	result := make([]APIKey, len(keys))
	for i, k := range keys {
		result[i] = APIKey{
			ID:        k.ID,
			ProjectID: k.ProjectID,
			Key:       k.Key,
			Name:      k.Name.String,
			CreatedAt: k.CreatedAt.Time.String(),
		}
	}

	writeJSON(w, http.StatusOK, result)
}

// CreateAPIKey handles POST /api/api-keys.
func (h *SettingsHandler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req CreateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	// Generate a random API key
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate API key")
		return
	}
	key := "pk-" + hex.EncodeToString(keyBytes)

	apiKey, err := h.queries.CreateAPIKey(r.Context(), db.CreateAPIKeyParams{
		ProjectID: projectID,
		Key:       key,
		Name:      pgtype.Text{String: req.Name, Valid: req.Name != ""},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create API key: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, APIKey{
		ID:        apiKey.ID,
		ProjectID: apiKey.ProjectID,
		Key:       apiKey.Key,
		Name:      apiKey.Name.String,
		CreatedAt: apiKey.CreatedAt.Time.String(),
	})
}

// DeleteAPIKey handles DELETE /api/api-keys/{id}.
func (h *SettingsHandler) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	err := h.queries.DeleteAPIKey(r.Context(), db.DeleteAPIKeyParams{
		ID:        id,
		ProjectID: projectID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete API key: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListMembers handles GET /api/members.
func (h *SettingsHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org_id")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	members, err := h.queries.GetMembersByOrgID(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list members: "+err.Error())
		return
	}

	result := make([]Member, len(members))
	for i, m := range members {
		result[i] = Member{
			ID:       m.ID,
			UserID:   m.UserID,
			OrgID:    m.OrgID,
			Role:     m.Role,
			Email:    m.Email,
			UserName: m.UserName.String,
		}
	}

	writeJSON(w, http.StatusOK, result)
}

// UpdateMemberRole handles PUT /api/members/{userId}/role.
func (h *SettingsHandler) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	var req UpdateMemberRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Role == "" {
		writeError(w, http.StatusBadRequest, "role is required")
		return
	}

	orgID := r.URL.Query().Get("org_id")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	err := h.queries.UpdateMemberRole(r.Context(), db.UpdateMemberRoleParams{
		UserID: userID,
		OrgID:  orgID,
		Role:   req.Role,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update member role: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// RemoveMember handles DELETE /api/members/{userId}.
func (h *SettingsHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	orgID := r.URL.Query().Get("org_id")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	err := h.queries.DeleteMember(r.Context(), db.DeleteMemberParams{
		UserID: userID,
		OrgID:  orgID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove member: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// GetProfile handles GET /api/profile.
func (h *SettingsHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserID(r.Context())

	user, err := h.queries.GetUserByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	writeJSON(w, http.StatusOK, UserWithProfile{
		ID:        user.ID,
		Email:     user.Email,
		Name:      user.Name.String,
		CreatedAt: user.CreatedAt.Time.String(),
	})
}

// UpdateProfile handles PUT /api/profile.
func (h *SettingsHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserID(r.Context())
	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	err := h.queries.UpdateUserName(r.Context(), db.UpdateUserNameParams{
		ID:   userID,
		Name: pgtype.Text{String: req.Name, Valid: req.Name != ""},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update profile: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// RegisterRoutes registers settings routes.
func (h *SettingsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api-keys", h.ListAPIKeys)
	r.Post("/api-keys", h.CreateAPIKey)
	r.Delete("/api-keys/{id}", h.DeleteAPIKey)
	r.Get("/members", h.ListMembers)
	r.Put("/members/{userId}/role", h.UpdateMemberRole)
	r.Delete("/members/{userId}", h.RemoveMember)
	r.Get("/profile", h.GetProfile)
	r.Put("/profile", h.UpdateProfile)
}
