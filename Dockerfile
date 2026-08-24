# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -a -installsuffix cgo \
    -ldflags="-w -s" \
    -o langfuse-light ./cmd/server

# Runtime stage — minimal image
FROM alpine:3.19

# Add non-root user for security
RUN addgroup -S langfuse && adduser -S langfuse -G langfuse

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy the binary and migrations from builder
COPY --from=builder /app/langfuse-light .
COPY --from=builder /app/migrations ./migrations

# Create data directory
RUN mkdir -p /app/data && chown -R langfuse:langfuse /app

USER langfuse

# Expose port
EXPOSE 3001

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:3001/health || exit 1

# Run the application
CMD ["./langfuse-light"]
