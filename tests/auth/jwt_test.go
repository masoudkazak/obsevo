package auth_test

import (
	"testing"
	"time"

	"github.com/obsevo/obsevo/internal/auth"
)

func TestJWTService_GenerateAndValidate(t *testing.T) {
	svc := auth.NewJWTService("test-secret-key", 1*time.Hour)

	token, err := svc.GenerateToken("user-123", "test@example.com")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	if token == "" {
		t.Fatal("expected non-empty token")
	}

	claims, err := svc.ValidateToken(token)
	if err != nil {
		t.Fatalf("failed to validate token: %v", err)
	}

	if claims.UserID != "user-123" {
		t.Errorf("expected UserID 'user-123', got %q", claims.UserID)
	}
	if claims.Email != "test@example.com" {
		t.Errorf("expected Email 'test@example.com', got %q", claims.Email)
	}
}

func TestJWTService_InvalidToken(t *testing.T) {
	svc := auth.NewJWTService("test-secret-key", 1*time.Hour)

	_, err := svc.ValidateToken("invalid-token-string")
	if err == nil {
		t.Fatal("expected error for invalid token, got nil")
	}
}

func TestJWTService_WrongSecret(t *testing.T) {
	svc1 := auth.NewJWTService("secret-1", 1*time.Hour)
	svc2 := auth.NewJWTService("secret-2", 1*time.Hour)

	token, err := svc1.GenerateToken("user-123", "test@example.com")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	_, err = svc2.ValidateToken(token)
	if err == nil {
		t.Fatal("expected error for token signed with different secret, got nil")
	}
}

func TestJWTService_EmptyToken(t *testing.T) {
	svc := auth.NewJWTService("test-secret-key", 1*time.Hour)

	_, err := svc.ValidateToken("")
	if err == nil {
		t.Fatal("expected error for empty token, got nil")
	}
}
