// Package log provides structured, context-aware logging for Obsevo.
//
// It wraps the standard library logger to automatically include a request ID
// (when present in the context) and key-value pairs, so every log line can be
// correlated to the HTTP request that triggered it.
package log

import (
	"context"
	"log"
	"strings"
)

type contextKey string

const requestIDKey contextKey = "request_id"

// WithRequestID stores a request ID in the context for use by Logger.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestID reads the request ID back out of a context.
func RequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// Logger writes structured log lines. Every method appends key=value pairs and
// prefixes the line with the request ID when one is available.
type Logger struct {
	ctx   context.Context
	extra []string
}

// From creates a logger bound to a context.
func From(ctx context.Context) *Logger {
	return &Logger{ctx: ctx}
}

// With appends key=value pairs to the logger.
func (l *Logger) With(pairs ...string) *Logger {
	cp := *l
	cp.extra = append(append([]string{}, l.extra...), pairs...)
	return &cp
}

func (l *Logger) emit(level, msg string) {
	var sb strings.Builder

	if id := RequestID(l.ctx); id != "" {
		sb.WriteString("request_id=")
		sb.WriteString(id)
		sb.WriteString(" ")
	}

	for i := 0; i+1 < len(l.extra); i += 2 {
		sb.WriteString(l.extra[i])
		sb.WriteString("=")
		sb.WriteString(l.extra[i+1])
		sb.WriteString(" ")
	}

	sb.WriteString("level=")
	sb.WriteString(level)
	sb.WriteString(" ")
	sb.WriteString(msg)

	log.Println(sb.String())
}

// Info logs an informational message.
func (l *Logger) Info(msg string, pairs ...string) {
	l.emit("info", msg)
}

// Error logs an error message.
func (l *Logger) Error(msg string, pairs ...string) {
	l.emit("error", msg)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, pairs ...string) {
	l.emit("warn", msg)
}

// Info is a convenience function for one-off info logs without a Logger instance.
func Info(msg string) {
	log.Println("level=info " + msg)
}

// Error is a convenience function for one-off error logs.
func Error(msg string) {
	log.Println("level=error " + msg)
}

// Warn is a convenience function for one-off warning logs.
func Warn(msg string) {
	log.Println("level=warn " + msg)
}
