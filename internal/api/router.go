package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/langfuse-light/langfuse-light/internal/auth"
	"github.com/langfuse-light/langfuse-light/internal/db"
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
	sdkHandler        *SDKHandler
}

// NewRouter creates a new API router with all handlers.
func NewRouter(queries *db.Queries, dbPool *pgxpool.Pool, jwtService *auth.JWTService, traceHandler *TraceHandler, promptHandler *PromptHandler, evaluationHandler *EvaluationHandler, datasetHandler *DatasetHandler, evaluatorHandler *EvaluatorHandler) *Router {
	return &Router{
		queries:           queries,
		dbPool:            dbPool,
		jwtService:        jwtService,
		authHandler:       NewAuthHandler(queries, jwtService),
		projectHandler:    NewProjectHandler(queries),
		traceHandler:      traceHandler,
		promptHandler:     promptHandler,
		evaluationHandler: evaluationHandler,
		datasetHandler:    datasetHandler,
		evaluatorHandler:  evaluatorHandler,
		settingsHandler:   NewSettingsHandler(queries),
		sdkHandler:        NewSDKHandler(traceHandler.traceService),
	}
}

// RegisterRoutes registers all API routes on the given chi.Router.
func (rt *Router) RegisterRoutes(r chi.Router) {
	// Health check — verifies DB connectivity
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		status := "ok"
		code := http.StatusOK

		if err := rt.dbPool.Ping(ctx); err != nil {
			status = "degraded"
			code = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  status,
			"service": "langfuse-light",
		})
	})

	// API routes
	r.Route("/api", func(r chi.Router) {
		// Public routes
		r.Route("/auth", rt.authHandler.RegisterRoutes)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(auth.Middleware(rt.jwtService))

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

			// Settings and management routes
			rt.settingsHandler.RegisterRoutes(r)
		})

		// SDK-compatible routes (API key auth)
		r.Route("/public", func(r chi.Router) {
			r.Use(auth.APIKeyMiddleware(rt.queries))
			rt.sdkHandler.RegisterSDKRoutes(r)
		})
	})
}
