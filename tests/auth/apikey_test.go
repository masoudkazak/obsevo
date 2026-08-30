package auth_test

import (
	"context"
	"strings"
	"testing"

	"github.com/obsevo/obsevo/internal/auth"
)

func TestGenerateKeyPairShape(t *testing.T) {
	pair, err := auth.GenerateKeyPair()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The Langfuse SDKs validate the key prefixes client-side.
	if !strings.HasPrefix(pair.PublicKey, "pk-lf-") {
		t.Errorf("expected a pk-lf- public key, got %q", pair.PublicKey)
	}
	if !strings.HasPrefix(pair.SecretKey, "sk-lf-") {
		t.Errorf("expected an sk-lf- secret key, got %q", pair.SecretKey)
	}

	// 32 random bytes rendered as hex, plus the prefix.
	if len(pair.PublicKey) != len("pk-lf-")+64 {
		t.Errorf("expected 32 bytes of entropy in the public key, got %d chars", len(pair.PublicKey))
	}
	if len(pair.SecretKey) != len("sk-lf-")+64 {
		t.Errorf("expected 32 bytes of entropy in the secret key, got %d chars", len(pair.SecretKey))
	}
}

func TestGenerateKeyPairIsUnique(t *testing.T) {
	seen := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		pair, err := auth.GenerateKeyPair()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, duplicate := seen[pair.PublicKey]; duplicate {
			t.Fatal("generated a duplicate public key")
		}
		seen[pair.PublicKey] = struct{}{}
	}
}

func TestKeyPairStoresOnlyADigest(t *testing.T) {
	pair, err := auth.GenerateKeyPair()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// What is persisted must not be the secret itself, or a database leak is a
	// credential leak.
	if pair.SecretKeyHash == pair.SecretKey {
		t.Fatal("the stored hash must not equal the secret key")
	}
	if strings.Contains(pair.SecretKeyHash, pair.SecretKey) {
		t.Fatal("the stored hash must not contain the secret key")
	}
	if len(pair.SecretKeyHash) != 64 {
		t.Errorf("expected a 64-character SHA-256 digest, got %d", len(pair.SecretKeyHash))
	}
	if got := auth.HashSecretKey(pair.SecretKey); got != pair.SecretKeyHash {
		t.Error("hashing the secret key again should reproduce the stored digest")
	}
}

func TestDisplaySecretRevealsOnlyTheTail(t *testing.T) {
	pair, err := auth.GenerateKeyPair()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasSuffix(pair.DisplaySecret, pair.SecretKey[len(pair.SecretKey)-4:]) {
		t.Errorf("expected the display key to end with the real tail, got %q", pair.DisplaySecret)
	}
	if len(pair.DisplaySecret) >= len(pair.SecretKey) {
		t.Errorf("expected the display key to be shorter than the secret, got %q", pair.DisplaySecret)
	}
}

func TestHashSecretKeyIsDeterministicAndDistinct(t *testing.T) {
	a := auth.HashSecretKey("sk-lf-a")
	b := auth.HashSecretKey("sk-lf-a")
	if a != b {
		t.Error("expected hashing to be deterministic")
	}
	if auth.HashSecretKey("sk-lf-a") == auth.HashSecretKey("sk-lf-b") {
		t.Error("expected different keys to hash differently")
	}
}

func TestRoleHierarchy(t *testing.T) {
	cases := []struct {
		role     string
		minimum  string
		expected bool
	}{
		{auth.RoleAdmin, auth.RoleViewer, true},
		{auth.RoleAdmin, auth.RoleEditor, true},
		{auth.RoleAdmin, auth.RoleAdmin, true},
		{auth.RoleEditor, auth.RoleViewer, true},
		{auth.RoleEditor, auth.RoleEditor, true},
		{auth.RoleEditor, auth.RoleAdmin, false},
		{auth.RoleViewer, auth.RoleViewer, true},
		{auth.RoleViewer, auth.RoleEditor, false},
		{auth.RoleViewer, auth.RoleAdmin, false},
	}

	for _, tc := range cases {
		ctx := context.WithValue(context.Background(), auth.RoleKey, tc.role)
		if got := auth.HasRole(ctx, tc.minimum); got != tc.expected {
			t.Errorf("%s meeting %s: expected %v, got %v", tc.role, tc.minimum, tc.expected, got)
		}
	}
}

func TestRoleIsDeniedWithoutAuthentication(t *testing.T) {
	if auth.HasRole(context.Background(), auth.RoleViewer) {
		t.Error("an unauthenticated context must not satisfy any role")
	}
}

func TestAPIKeyRequestsAreTreatedAsEditor(t *testing.T) {
	// Ingestion needs to write, but an API key must never carry admin rights.
	ctx := context.WithValue(context.Background(), auth.APIKeyIDKey, "key-1")

	if !auth.HasRole(ctx, auth.RoleEditor) {
		t.Error("expected an API key to satisfy EDITOR")
	}
	if auth.HasRole(ctx, auth.RoleAdmin) {
		t.Error("expected an API key not to satisfy ADMIN")
	}
}
