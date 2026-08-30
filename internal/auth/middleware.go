package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

type contextKey string

const (
	UserIDKey    contextKey = "user_id"
	EmailKey     contextKey = "email"
	ProjectIDKey contextKey = "project_id"
	APIKeyIDKey  contextKey = "api_key_id"
)

// Middleware validates JWT tokens from Authorization header
func Middleware(jwtService *JWTService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				http.Error(w, `{"error":"invalid authorization header format"}`, http.StatusUnauthorized)
				return
			}

			claims, err := jwtService.ValidateToken(parts[1])
			if err != nil {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, EmailKey, claims.Email)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserID extracts user ID from context
func GetUserID(ctx context.Context) string {
	if userID, ok := ctx.Value(UserIDKey).(string); ok {
		return userID
	}
	return ""
}

// GetEmail extracts email from context
func GetEmail(ctx context.Context) string {
	if email, ok := ctx.Value(EmailKey).(string); ok {
		return email
	}
	return ""
}

// GetProjectID extracts project ID from context (set by API key middleware).
func GetProjectID(ctx context.Context) string {
	if pid, ok := ctx.Value(ProjectIDKey).(string); ok {
		return pid
	}
	return ""
}

// GetAPIKeyID extracts API key ID from context (set by API key middleware).
func GetAPIKeyID(ctx context.Context) string {
	if id, ok := ctx.Value(APIKeyIDKey).(string); ok {
		return id
	}
	return ""
}

// pgText wraps a string for a nullable text query parameter.
func pgText(v string) pgtype.Text {
	return pgtype.Text{String: v, Valid: v != ""}
}
