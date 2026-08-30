package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/langfuse-light/langfuse-light/internal/auth"
	"github.com/langfuse-light/langfuse-light/internal/db"
)

// CORS returns a middleware that answers preflight requests and sets the
// cross-origin headers.
//
// With no configured origins the API is same-origin only: no CORS headers are
// emitted, so a browser on another origin cannot read responses. That is the
// right default for a self-hosted deployment where the SPA is served from the
// same host. Set CORS_ALLOWED_ORIGINS to open it up deliberately.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowAll := false
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		if origin == "*" {
			allowAll = true
		}
		allowed[origin] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if origin != "" && (allowAll || contains(allowed, origin)) {
				if allowAll {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					// Credentials are only safe with an exact origin echo, never with "*".
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Add("Vary", "Origin")
				}
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, x-api-key, X-Project-Id")
				w.Header().Set("Access-Control-Max-Age", "600")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func contains(set map[string]struct{}, key string) bool {
	_, ok := set[key]
	return ok
}

// SecurityHeaders sets the response headers that cost nothing and close off
// the common browser-side attacks against the dashboard.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// RateLimiter throttles requests using a fixed window counter in Redis.
//
// A fixed window is chosen over a sliding log because it costs one INCR per
// request and no per-request storage — the difference matters on the ingestion
// path, and the burst a fixed window permits at a boundary is not a concern for
// the abuse this guards against (credential stuffing, runaway clients).
type RateLimiter struct {
	client    *redis.Client
	perMinute int
}

// NewRateLimiter creates a rate limiter. A limit of zero disables throttling.
func NewRateLimiter(client *redis.Client, perMinute int) *RateLimiter {
	return &RateLimiter{client: client, perMinute: perMinute}
}

// Limit returns a middleware enforcing a per-scope limit. Each scope counts
// separately, so a burst of ingestion cannot lock a user out of logging in.
//
// Rate limiting is off unless RATE_LIMIT_PER_MINUTE is set: a self-hosted
// single-tenant deployment usually wants no throttle at all, and a limiter that
// switched itself on would cap ingestion for everyone who never asked for it.
// When it is enabled, scopeLimit gives each route group a ceiling appropriate
// to its traffic, falling back to the configured global value.
func (rl *RateLimiter) Limit(scope string, scopeLimit int) func(http.Handler) http.Handler {
	enabled := rl != nil && rl.client != nil && rl.perMinute > 0

	perMinute := scopeLimit
	if perMinute <= 0 {
		perMinute = rl.perMinute
	}

	return func(next http.Handler) http.Handler {
		if !enabled {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := fmt.Sprintf("langfuse:ratelimit:%s:%s:%d",
				scope, rateLimitSubject(r), time.Now().Unix()/60)

			count, err := rl.client.Incr(r.Context(), key).Result()
			if err != nil {
				// Redis being unavailable must not take the API down with it;
				// failing open is the right trade for a throttle.
				next.ServeHTTP(w, r)
				return
			}
			if count == 1 {
				rl.client.Expire(r.Context(), key, 2*time.Minute)
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(perMinute))
			remaining := perMinute - int(count)
			if remaining < 0 {
				remaining = 0
			}
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

			if int(count) > perMinute {
				w.Header().Set("Retry-After", "60")
				writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// rateLimitSubject identifies who a request is counted against: the API key
// first, then the authenticated user, then the client address.
func rateLimitSubject(r *http.Request) string {
	if keyID := auth.GetAPIKeyID(r.Context()); keyID != "" {
		return "key:" + keyID
	}
	if userID := auth.GetUserID(r.Context()); userID != "" {
		return "user:" + userID
	}
	return "ip:" + clientIP(r)
}

// clientIP extracts the caller's address, honouring a single proxy hop.
func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if idx := strings.IndexByte(forwarded, ','); idx > 0 {
			return strings.TrimSpace(forwarded[:idx])
		}
		return strings.TrimSpace(forwarded)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// AuditLogger records security-relevant mutations.
type AuditLogger struct {
	queries *db.Queries
}

// NewAuditLogger creates an audit logger.
func NewAuditLogger(queries *db.Queries) *AuditLogger {
	return &AuditLogger{queries: queries}
}

// Record writes one audit entry. Audit logging must never fail the operation it
// is recording, so errors are swallowed after the write is attempted.
func (a *AuditLogger) Record(ctx context.Context, r *http.Request, action, resource, resourceID string) {
	if a == nil || a.queries == nil {
		return
	}

	_ = a.queries.CreateAuditLog(ctx, db.CreateAuditLogParams{
		ProjectID:  auth.GetProjectID(ctx),
		UserID:     auth.GetUserID(ctx),
		ApiKeyID:   auth.GetAPIKeyID(ctx),
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		IpAddress:  clientIP(r),
	})
}

// AuditMutations returns a middleware that records every state-changing request
// on the routes it wraps. Reads are not recorded: they would dominate the table
// while answering none of the questions an audit log exists to answer.
func (a *AuditLogger) AuditMutations(resource string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
				next.ServeHTTP(recorder, r)

				// Only successful mutations are worth recording; a rejected
				// request changed nothing.
				if recorder.status >= 200 && recorder.status < 300 {
					a.Record(r.Context(), r, r.Method, resource, r.URL.Path)
				}
			default:
				next.ServeHTTP(w, r)
			}
		})
	}
}

// statusRecorder captures the response status so the audit entry can be written
// only for requests that actually succeeded.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (rec *statusRecorder) WriteHeader(status int) {
	if !rec.wroteHeader {
		rec.status = status
		rec.wroteHeader = true
	}
	rec.ResponseWriter.WriteHeader(status)
}

func (rec *statusRecorder) Write(b []byte) (int, error) {
	if !rec.wroteHeader {
		rec.wroteHeader = true
	}
	return rec.ResponseWriter.Write(b)
}
