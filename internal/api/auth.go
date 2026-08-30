package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/obsevo/obsevo/internal/auth"
	"github.com/obsevo/obsevo/internal/db"
)

// AuthHandler handles authentication requests
type AuthHandler struct {
	queries    *db.Queries
	jwtService *auth.JWTService
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(queries *db.Queries, jwtService *auth.JWTService) *AuthHandler {
	return &AuthHandler{
		queries:    queries,
		jwtService: jwtService,
	}
}

// RegisterRequest represents the register request body
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// LoginRequest represents the login request body
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResponse represents the auth response
type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// User represents a user in responses
type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// Register handles user registration
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	// Check if user already exists
	ctx := r.Context()
	existingUser, _ := h.queries.GetUserByEmail(ctx, req.Email)
	if existingUser.ID != "" {
		writeError(w, http.StatusConflict, "user with this email already exists")
		return
	}

	// Hash password
	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	// Create user
	user, err := h.queries.CreateUser(ctx, db.CreateUserParams{
		Email:        req.Email,
		PasswordHash: hashedPassword,
		Name:         pgtype.Text{String: req.Name, Valid: req.Name != ""},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	// Auto-create default organization, member, and project
	org, err := h.queries.CreateOrganization(ctx, "My Organization")
	if err == nil {
		_, _ = h.queries.CreateMember(ctx, db.CreateMemberParams{
			UserID: user.ID,
			OrgID:  org.ID,
			Role:   "ADMIN",
		})
		projects, _ := h.queries.GetProjectsByOrgID(ctx, org.ID)
		if len(projects) == 0 {
			_, _ = h.queries.CreateProject(ctx, db.CreateProjectParams{Name: "My Project", OrgID: org.ID})
		}
	}

	// Generate JWT token
	token, err := h.jwtService.GenerateToken(user.ID, user.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	writeJSON(w, http.StatusCreated, AuthResponse{
		Token: token,
		User: User{
			ID:    user.ID,
			Email: user.Email,
			Name:  user.Name.String,
		},
	})
}

// Login handles user login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	// Get user by email
	user, err := h.queries.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	// Check password
	if !auth.CheckPassword(req.Password, user.PasswordHash) {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	// Generate JWT token
	token, err := h.jwtService.GenerateToken(user.ID, user.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	writeJSON(w, http.StatusOK, AuthResponse{
		Token: token,
		User: User{
			ID:    user.ID,
			Email: user.Email,
			Name:  user.Name.String,
		},
	})
}

// RegisterRoutes registers auth routes
func (h *AuthHandler) RegisterRoutes(r chi.Router) {
	r.Post("/register", h.Register)
	r.Post("/login", h.Login)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
