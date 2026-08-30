.PHONY: all build run test lint fmt clean docker-up docker-down db-migrate db-rollback sqlc-generate benchmark

# Variables
APP_NAME := obsevo
BUILD_DIR := ./bin
GO := go
GOFLAGS := -v
SQLC := sqlc

# Default target
all: build

# Build the Go binary
build:
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(APP_NAME) ./cmd/server

# Run the application
run:
	$(GO) run ./cmd/server

# Run tests
test:
	$(GO) test -v ./...

# Run tests with coverage
test-coverage:
	$(GO) test -v -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

# Run linter
lint:
	golangci-lint run ./...

# Format code
fmt:
	gofmt -w .

# Check formatting
fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Code not formatted. Run 'make fmt'" && exit 1)

# Vet code
vet:
	$(GO) vet ./...

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Docker commands
docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f

# Database commands
db-migrate:
	@echo "Run migrations using migrate tool:"
	@echo "migrate -path migrations -database \"$(DATABASE_URL)\" up"

db-rollback:
	@echo "Rollback using migrate tool:"
	@echo "migrate -path migrations -database \"$(DATABASE_URL)\" down 1"

# sqlc commands
sqlc-generate:
	$(SQLC) generate

# Development setup
dev-setup: docker-up sqlc-generate
	@echo "Development environment ready!"

# Install tools (run once)
install-tools:
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Help
help:
	@echo "Available commands:"
	@echo "  make build        - Build the Go binary"
	@echo "  make run          - Run the application"
	@echo "  make test         - Run tests"
	@echo "  make test-coverage - Run tests with coverage report"
	@echo "  make lint         - Run linter"
	@echo "  make fmt          - Format code"
	@echo "  make fmt-check    - Check code formatting"
	@echo "  make vet          - Run go vet"
	@echo "  make clean        - Clean build artifacts"
	@echo "  make docker-up    - Start Docker containers"
	@echo "  make docker-down  - Stop Docker containers"
	@echo "  make docker-logs  - View Docker logs"
	@echo "  make db-migrate   - Run database migrations"
	@echo "  make db-rollback  - Rollback last migration"
	@echo "  make sqlc-generate - Generate sqlc code"
	@echo "  make dev-setup    - Full development setup"
	@echo "  make install-tools - Install development tools"
	@echo "  make help         - Show this help"

# Benchmark: runs the scenarios reported in docs/benchmark.md.
# Requires the stack to be up and LANGFUSE_PUBLIC_KEY / LANGFUSE_SECRET_KEY set.
benchmark:
	@test -n "$$LANGFUSE_PUBLIC_KEY" || (echo "set LANGFUSE_PUBLIC_KEY and LANGFUSE_SECRET_KEY first" && exit 1)
	@for rate in 100 1000 10000 60000; do \
		echo "=== $$rate traces/min ==="; \
		$(GO) run ./cmd/benchmark -rate $$rate -duration 30s -observations 4 || exit 1; \
	done
