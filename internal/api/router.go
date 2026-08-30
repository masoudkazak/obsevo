package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/redis/go-redis/v9"

	"github.com/obsevo/obsevo/internal/auth"
	"github.com/obsevo/obsevo/internal/config"
	"github.com/obsevo/obsevo/internal/db"
	"github.com/obsevo/obsevo/internal/queue"
	"github.com/obsevo/obsevo/internal/services"
)

// Router holds all handlers and configures the API routes.
type Router struct {
	queries           *db.Queries
	dbPool            *pgxpool.Pool
	jwtService        *auth.JWTService
	authHandler       *AuthHandler
	projectHandler    *ProjectHandler
	traceHandler      *TraceHandler
	promptHandler     *PromptHandler
	evaluationHandler *EvaluationHandler
	datasetHandler    *DatasetHandler
	evaluatorHandler  *EvaluatorHandler
	settingsHandler   *SettingsHandler
	modelPriceHandler *ModelPriceHandler
	sdkHandler        *SDKHandler
	rateLimiter       *RateLimiter
	auditLogger       *AuditLogger
	corsOrigins       []string
}

// RouterDeps carries the handlers and services the router wires together.
type RouterDeps struct {
	Queries    *db.Queries
	DBPool     *pgxpool.Pool
	JWTService *auth.JWTService
	Queue      *queue.Queue
	Redis      *redis.Client
	Config     *config.Config

	TraceHandler      *TraceHandler
	PromptHandler     *PromptHandler
	EvaluationHandler *EvaluationHandler
	DatasetHandler    *DatasetHandler
	EvaluatorHandler  *EvaluatorHandler

	EvaluationService *services.EvaluationService
	PromptService     *services.PromptService
	DatasetService    *services.DatasetService
	CostService       *services.CostService
}

// NewRouter creates a new API router with all handlers.
func NewRouter(deps RouterDeps) *Router {
	return &Router{
		queries:           deps.Queries,
		dbPool:            deps.DBPool,
		jwtService:        deps.JWTService,
		authHandler:       NewAuthHandler(deps.Queries, deps.JWTService),
		projectHandler:    NewProjectHandler(deps.Queries),
		traceHandler:      deps.TraceHandler,
		promptHandler:     deps.PromptHandler,
		evaluationHandler: deps.EvaluationHandler,
		datasetHandler:    deps.DatasetHandler,
		evaluatorHandler:  deps.EvaluatorHandler,
		settingsHandler:   NewSettingsHandler(deps.Queries),
		modelPriceHandler: NewModelPriceHandler(deps.CostService),
		sdkHandler:        NewSDKHandler(deps.TraceHandler.traceService, deps.EvaluationService, deps.PromptService, deps.DatasetService, deps.Queue),
		rateLimiter:       NewRateLimiter(deps.Redis, rateLimitPerMinute(deps.Config)),
		auditLogger:       NewAuditLogger(deps.Queries),
		corsOrigins:       corsOrigins(deps.Config),
	}
}

// rateLimitPerMinute reads the configured limit, tolerating a nil config in
// tests that construct a router directly.
func rateLimitPerMinute(cfg *config.Config) int {
	if cfg == nil {
		return 0
	}
	return cfg.RateLimitPerMinute
}

func corsOrigins(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	return cfg.CORSAllowedOrigins
}

// RegisterRoutes registers all API routes on the given chi.Router.
func (rt *Router) RegisterRoutes(r chi.Router) {
	// Health check — verifies DB connectivity
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		status := "ok"
		code := http.StatusOK

		if err := rt.dbPool.Ping(ctx); err != nil {
			status = "degraded"
			code = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  status,
			"service": "obsevo",
		})
	})

	// API routes
	r.Route("/api", func(r chi.Router) {
		r.Use(CORS(rt.corsOrigins))
		r.Use(SecurityHeaders)

		// Credential endpoints are throttled hardest: they are the ones worth
		// guessing at, and a legitimate client calls them rarely.
		r.Group(func(r chi.Router) {
			r.Use(rt.rateLimiter.Limit("auth", 20))
			r.Route("/auth", rt.authHandler.RegisterRoutes)
		})

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(auth.Middleware(rt.jwtService))
			r.Use(rt.rateLimiter.Limit("dashboard", 600))
			// Authorizes `project_id` once, so handlers can trust the project
			// they read from the request context.
			r.Use(auth.ProjectMiddleware(rt.queries))

			// Project and organization routes
			rt.projectHandler.RegisterRoutes(r)

			// Trace and observation routes
			rt.traceHandler.RegisterRoutes(r)

			// Prompt routes
			rt.promptHandler.RegisterRoutes(r)

			// Evaluation and analytics routes
			rt.evaluationHandler.RegisterRoutes(r)

			// Dataset routes
			rt.datasetHandler.RegisterRoutes(r)

			// Evaluator routes
			rt.evaluatorHandler.RegisterRoutes(r)

			// Settings and management routes. API keys and membership are the
			// changes worth being able to reconstruct after the fact.
			r.Group(func(r chi.Router) {
				r.Use(rt.auditLogger.AuditMutations("settings"))
				rt.settingsHandler.RegisterRoutes(r)
			})

			// Model pricing routes
			rt.modelPriceHandler.RegisterRoutes(r)
		})

		// SDK-compatible routes (API key auth). The ingestion limit is high
		// because a single SDK flush is one request carrying many events.
		r.Route("/public", func(r chi.Router) {
			r.Use(auth.APIKeyMiddleware(rt.queries))
			r.Use(rt.rateLimiter.Limit("ingestion", 6000))
			rt.sdkHandler.RegisterSDKRoutes(r)
		})
	})
}
