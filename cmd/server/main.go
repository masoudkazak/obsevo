package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/langfuse-light/langfuse-light/internal/api"
	"github.com/langfuse-light/langfuse-light/internal/auth"
	"github.com/langfuse-light/langfuse-light/internal/config"
	"github.com/langfuse-light/langfuse-light/internal/db"
	"github.com/langfuse-light/langfuse-light/internal/queue"
	"github.com/langfuse-light/langfuse-light/internal/services"
	"github.com/langfuse-light/langfuse-light/internal/worker"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Log startup info
	log.Printf("Starting Langfuse Light in %s mode", cfg.AppEnv)

	// Connect to PostgreSQL
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}
	defer pool.Close()

	// Test database connection
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("Unable to ping database: %v", err)
	}
	log.Println("Connected to PostgreSQL")

	// Connect to Redis
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.RedisURL,
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Unable to connect to Redis: %v", err)
	}
	log.Println("Connected to Redis")

	// Initialize queries
	queries := db.New(pool)

	// Initialize JWT service
	jwtService := auth.NewJWTService(cfg.JWTSecret, 24*time.Hour)

	// Initialize services
	traceService := services.NewTraceService(queries)
	promptService := services.NewPromptService(queries)
	evalService := services.NewEvaluationService(queries)
	datasetService := services.NewDatasetService(queries)

	// Initialize queue and worker
	ingestionQueue := queue.NewQueue(rdb)
	ingestionWorker := worker.NewWorker(ingestionQueue, traceService, 1*time.Second)

	// Start background worker
	go ingestionWorker.Start(ctx)

	// Initialize handlers
	traceHandler := api.NewTraceHandler(traceService)
	promptHandler := api.NewPromptHandler(promptService)
	evalHandler := api.NewEvaluationHandler(evalService)
	datasetHandler := api.NewDatasetHandler(datasetService)

	// Setup router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Register all routes
	apiRouter := api.NewRouter(queries, jwtService, traceHandler, promptHandler, evalHandler, datasetHandler)
	apiRouter.RegisterRoutes(r)

	// Start server
	addr := fmt.Sprintf(":%d", cfg.AppPort)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		log.Printf("Server listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Stop the worker
	ingestionWorker.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}
