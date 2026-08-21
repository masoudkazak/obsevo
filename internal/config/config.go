package config

import (
	"os"
	"strconv"
)

// Config holds all configuration for the application
type Config struct {
	// Application
	AppEnv    string
	AppPort   int
	AppSecret string

	// Database
	DatabaseURL string

	// Redis
	RedisURL string

	// JWT
	JWTSecret string
	JWTExpiry string

	// Storage
	UploadDir     string
	MaxUploadSize int64

	// Logging
	LogLevel  string
	LogFormat string
}

// Load loads configuration from environment variables
func Load() *Config {
	return &Config{
		AppEnv:        getEnv("APP_ENV", "development"),
		AppPort:       getEnvAsInt("APP_PORT", 3001),
		AppSecret:     getEnv("APP_SECRET_KEY", "change-me-in-production"),
		DatabaseURL:   getEnv("DATABASE_URL", "postgres://langfuse:langfuse@localhost:5432/langfuse_light?sslmode=disable"),
		RedisURL:      getEnv("REDIS_URL", "redis://localhost:6379"),
		JWTSecret:     getEnv("JWT_SECRET", "change-me-in-production"),
		JWTExpiry:     getEnv("JWT_EXPIRY", "24h"),
		UploadDir:     getEnv("UPLOAD_DIR", "./data/uploads"),
		MaxUploadSize: getEnvAsInt64("MAX_UPLOAD_SIZE", 10485760),
		LogLevel:      getEnv("LOG_LEVEL", "debug"),
		LogFormat:     getEnv("LOG_FORMAT", "json"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvAsInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intVal
		}
	}
	return defaultValue
}
