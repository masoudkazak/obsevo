package auth

import (
	"context"
	"net/http"

	"github.com/langfuse-light/langfuse-light/internal/db"
)

// RoleKey holds the caller's role in the organization owning the current project.
const RoleKey contextKey = "member_role"

// Role names, ordered from least to most privileged.
const (
	RoleViewer = "VIEWER"
	RoleEditor = "EDITOR"
	RoleAdmin  = "ADMIN"
)

var roleRank = map[string]int{
	RoleViewer: 1,
	RoleEditor: 2,
	RoleAdmin:  3,
}

// ProjectMiddleware authorizes the project a JWT-authenticated request targets.
//
// The project is named by the `project_id` query parameter or the
// `X-Project-Id` header. When neither is present the request is passed through
// untouched — routes such as /api/projects and /api/profile are not scoped to a
// project, and handlers that do need one reject the empty value themselves.
// When a project is named, the caller must belong to the organization that owns
// it; otherwise the request is refused. Handlers therefore never have to
// re-check the project id they read from the context.
func ProjectMiddleware(queries *db.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			projectID := r.URL.Query().Get("project_id")
			if projectID == "" {
				projectID = r.Header.Get("X-Project-Id")
			}
			if projectID == "" {
				next.ServeHTTP(w, r)
				return
			}

			userID := GetUserID(r.Context())
			if userID == "" {
				http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
				return
			}

			role, err := queries.GetMemberRoleForProject(r.Context(), db.GetMemberRoleForProjectParams{
				ID:     projectID,
				UserID: userID,
			})
			if err != nil {
				// Not found and not-a-member are deliberately indistinguishable,
				// so project ids cannot be enumerated through this endpoint.
				http.Error(w, `{"error":"project not found"}`, http.StatusNotFound)
				return
			}

			ctx := context.WithValue(r.Context(), ProjectIDKey, projectID)
			ctx = context.WithValue(ctx, RoleKey, role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetRole extracts the caller's project role from context.
func GetRole(ctx context.Context) string {
	if role, ok := ctx.Value(RoleKey).(string); ok {
		return role
	}
	return ""
}

// HasRole reports whether the caller's role meets the required minimum.
// Requests authenticated by API key carry no role and are treated as EDITOR,
// which is what ingestion needs and no more.
func HasRole(ctx context.Context, minimum string) bool {
	role := GetRole(ctx)
	if role == "" {
		if GetAPIKeyID(ctx) != "" {
			role = RoleEditor
		} else {
			return false
		}
	}
	return roleRank[role] >= roleRank[minimum]
}

// RequireRole rejects requests whose caller does not meet the minimum role.
func RequireRole(minimum string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !HasRole(r.Context(), minimum) {
				http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
