package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/obsevo/obsevo/internal/api"
	"github.com/obsevo/obsevo/internal/auth"
	"github.com/obsevo/obsevo/internal/config"
	"github.com/obsevo/obsevo/internal/db"
	"github.com/obsevo/obsevo/internal/queue"
	"github.com/obsevo/obsevo/internal/services"
	"github.com/obsevo/obsevo/internal/worker"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Log startup info
	log.Printf("Starting Langfuse Light in %s mode", cfg.AppEnv)

	// Connect to PostgreSQL
	ctx := context.Background()
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Invalid DATABASE_URL: %v", err)
	}
	// The ingestion shards and the HTTP handlers share this pool; sizing it
	// only for request concurrency starves the worker under load.
	if cfg.DBMaxConns > 0 {
		poolConfig.MaxConns = cfg.DBMaxConns
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}
	defer pool.Close()

	// Test database connection
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("Unable to ping database: %v", err)
	}
	log.Println("Connected to PostgreSQL")

	// Run migrations
	migrationsDir := os.Getenv("MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "./migrations"
	}
	if err := runMigrations(ctx, pool, migrationsDir); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

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
	costService := services.NewCostService(queries)
	experimentService := services.NewExperimentService(queries)
	evaluatorService := services.NewEvaluatorConfigService(queries, cfg.EvaluatorTimeout)

	// Initialize queue and worker
	ingestionQueue := queue.NewQueue(rdb)
	ingestionWorker := worker.NewWorker(ingestionQueue, traceService, evalService, costService, cfg.WorkerConcurrency)

	// The worker owns its own cancellable context so shutdown can unblock the
	// Redis read without tearing down in-flight HTTP requests.
	workerCtx, stopWorker := context.WithCancel(ctx)
	defer stopWorker()
	go ingestionWorker.Start(workerCtx)

	// Initialize handlers
	traceHandler := api.NewTraceHandler(traceService)
	promptHandler := api.NewPromptHandler(promptService)
	evalHandler := api.NewEvaluationHandler(evalService)
	datasetHandler := api.NewDatasetHandler(datasetService, experimentService)
	evaluatorHandler := api.NewEvaluatorHandler(evaluatorService)

	// Setup router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Register all routes
	apiRouter := api.NewRouter(api.RouterDeps{
		Queries:           queries,
		DBPool:            pool,
		JWTService:        jwtService,
		Queue:             ingestionQueue,
		Redis:             rdb,
		Config:            cfg,
		TraceHandler:      traceHandler,
		PromptHandler:     promptHandler,
		EvaluationHandler: evalHandler,
		DatasetHandler:    datasetHandler,
		EvaluatorHandler:  evaluatorHandler,
		EvaluationService: evalService,
		PromptService:     promptService,
		DatasetService:    datasetService,
		CostService:       costService,
	})
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

	// Stop the worker and let it drain the item it is holding.
	stopWorker()
	ingestionWorker.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}

// runMigrations executes all .up.sql migration files in order, skipping already applied ones.
func runMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	// Create migrations tracking table
	_, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		filename TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`)
	if err != nil {
		return fmt.Errorf("creating schema_migrations table: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading migrations dir: %w", err)
	}

	var upFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			upFiles = append(upFiles, e.Name())
		}
	}
	sort.Strings(upFiles)

	for _, name := range upFiles {
		// Skip already applied migrations
		var exists bool
		err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename = $1)`, name).Scan(&exists)
		if err != nil {
			return fmt.Errorf("checking migration %s: %w", name, err)
		}
		if exists {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("executing migration %s: %w", name, err)
		}

		_, err = pool.Exec(ctx, `INSERT INTO schema_migrations (filename) VALUES ($1)`, name)
		if err != nil {
			return fmt.Errorf("recording migration %s: %w", name, err)
		}
		log.Printf("Applied migration: %s", name)
	}
	return nil
}
