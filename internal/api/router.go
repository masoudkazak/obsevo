package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/langfuse-light/langfuse-light/internal/auth"
	"github.com/langfuse-light/langfuse-light/internal/db"
)

// Router holds all handlers and configures the API routes.
type Router struct {
	queries           *db.Queries
	jwtService        *auth.JWTService
	authHandler       *AuthHandler
	projectHandler    *ProjectHandler
	traceHandler      *TraceHandler
	promptHandler     *PromptHandler
	evaluationHandler *EvaluationHandler
	datasetHandler    *DatasetHandler
	settingsHandler   *SettingsHandler
	sdkHandler        *SDKHandler
}

// NewRouter creates a new API router with all handlers.
func NewRouter(queries *db.Queries, jwtService *auth.JWTService, traceHandler *TraceHandler, promptHandler *PromptHandler, evaluationHandler *EvaluationHandler, datasetHandler *DatasetHandler) *Router {
	return &Router{
		queries:           queries,
		jwtService:        jwtService,
		authHandler:       NewAuthHandler(queries, jwtService),
		projectHandler:    NewProjectHandler(queries),
		traceHandler:      traceHandler,
		promptHandler:     promptHandler,
		evaluationHandler: evaluationHandler,
		datasetHandler:    datasetHandler,
		settingsHandler:   NewSettingsHandler(queries),
		sdkHandler:        NewSDKHandler(traceHandler.traceService),
	}
}

// RegisterRoutes registers all API routes on the given chi.Router.
func (rt *Router) RegisterRoutes(r chi.Router) {
	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// API routes
	r.Route("/api", func(r chi.Router) {
		// Public routes
		r.Route("/auth", rt.authHandler.RegisterRoutes)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(auth.Middleware(rt.jwtService))

			// Project and organization routes
			r.Route("/", rt.projectHandler.RegisterRoutes)

			// Trace and observation routes
			r.Route("/", rt.traceHandler.RegisterRoutes)

			// Prompt routes
			r.Route("/", rt.promptHandler.RegisterRoutes)

			// Evaluation and analytics routes
			r.Route("/", rt.evaluationHandler.RegisterRoutes)

			// Dataset routes
			r.Route("/", rt.datasetHandler.RegisterRoutes)

			// Settings and management routes
			r.Route("/", rt.settingsHandler.RegisterRoutes)
		})

		// SDK-compatible routes (API key auth)
		r.Route("/public", func(r chi.Router) {
			r.Use(auth.APIKeyMiddleware(rt.queries))
			r.Route("/", rt.sdkHandler.RegisterSDKRoutes)
		})
	})
}
