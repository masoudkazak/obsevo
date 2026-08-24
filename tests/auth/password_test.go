package auth_test

import (
	"testing"

	"github.com/langfuse-light/langfuse-light/internal/auth"
)

func TestHashPassword(t *testing.T) {
	hash, err := auth.HashPassword("mypassword")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

	if hash == "mypassword" {
		t.Fatal("hash should not equal plaintext password")
	}
}

func TestCheckPassword_Correct(t *testing.T) {
	hash, err := auth.HashPassword("correct-password")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	if !auth.CheckPassword("correct-password", hash) {
		t.Error("expected correct password to match")
	}
}

func TestCheckPassword_Incorrect(t *testing.T) {
	hash, err := auth.HashPassword("correct-password")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	if auth.CheckPassword("wrong-password", hash) {
		t.Error("expected wrong password to not match")
	}
}

func TestCheckPassword_EmptyPassword(t *testing.T) {
	hash, err := auth.HashPassword("")
	if err != nil {
		t.Fatalf("failed to hash empty password: %v", err)
	}

	if !auth.CheckPassword("", hash) {
		t.Error("expected empty password to match its hash")
	}
}
