package log_test

import (
	"context"
	"testing"

	"github.com/obsevo/obsevo/internal/log"
)

func TestWithRequestID_And_RequestID(t *testing.T) {
	ctx := log.WithRequestID(context.Background(), "req-abc-123")
	got := log.RequestID(ctx)
	if got != "req-abc-123" {
		t.Errorf("expected 'req-abc-123', got %q", got)
	}
}

func TestRequestID_Empty(t *testing.T) {
	got := log.RequestID(context.Background())
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestLogger_With(t *testing.T) {
	ctx := log.WithRequestID(context.Background(), "req-1")
	l := log.From(ctx).With("key", "value")

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Logger.With panicked: %v", r)
		}
	}()

	l.Info("test message")
}

func TestLogger_Error(t *testing.T) {
	l := log.From(context.Background())
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Logger.Error panicked: %v", r)
		}
	}()

	l.Error("test error")
}

func TestLogger_Warn(t *testing.T) {
	l := log.From(context.Background())
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Logger.Warn panicked: %v", r)
		}
	}()

	l.Warn("test warning")
}

func TestConvenienceFunctions(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("convenience functions panicked: %v", r)
		}
	}()

	log.Info("info message")
	log.Error("error message")
	log.Warn("warn message")
}
