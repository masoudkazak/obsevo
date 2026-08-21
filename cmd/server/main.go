package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/langfuse-light/langfuse-light/internal/config"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Log startup info
	log.Printf("Starting Langfuse Light in %s mode", cfg.AppEnv)
	log.Printf("Listening on port %d", cfg.AppPort)

	// Simple health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok"}`)
	})

	// Start server
	addr := fmt.Sprintf(":%d", cfg.AppPort)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
