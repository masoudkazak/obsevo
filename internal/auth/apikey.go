package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/obsevo/obsevo/internal/db"
)

// Key prefixes match Langfuse, so a key is recognisable at a glance and the
// SDKs' own validation of the public key format is satisfied.
const (
	publicKeyPrefix = "pk-lf-"
	secretKeyPrefix = "sk-lf-"
)

// apiKeyCacheTTL bounds how long a verified key is trusted without a database
// round trip. Ingestion authenticates on every request, so this is the
// difference between one query per event and one query per key per minute.
const apiKeyCacheTTL = 60 * time.Second

// KeyPair is a freshly generated credential. The secret is returned exactly
// once, at creation; only its digest is stored.
type KeyPair struct {
	PublicKey     string
	SecretKey     string
	SecretKeyHash string
	DisplaySecret string
}

// GenerateKeyPair creates a Langfuse-style public/secret key pair.
func GenerateKeyPair() (KeyPair, error) {
	public, err := randomToken()
	if err != nil {
		return KeyPair{}, fmt.Errorf("generating public key: %w", err)
	}
	secret, err := randomToken()
	if err != nil {
		return KeyPair{}, fmt.Errorf("generating secret key: %w", err)
	}

	secretKey := secretKeyPrefix + secret
	return KeyPair{
		PublicKey:     publicKeyPrefix + public,
		SecretKey:     secretKey,
		SecretKeyHash: HashSecretKey(secretKey),
		DisplaySecret: maskSecret(secretKey),
	}, nil
}

// HashSecretKey digests a secret key for storage.
//
// SHA-256 rather than bcrypt is deliberate: an API key is 256 bits of
// generated entropy, not a user-chosen password, so there is nothing for a
// slow KDF to protect against — brute-forcing the keyspace is infeasible
// regardless — while bcrypt's cost would be paid on every ingestion request.
func HashSecretKey(secretKey string) string {
	sum := sha256.Sum256([]byte(secretKey))
	return hex.EncodeToString(sum[:])
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// maskSecret renders the tail of a secret for display in the UI.
func maskSecret(secretKey string) string {
	if len(secretKey) <= len(secretKeyPrefix)+4 {
		return secretKeyPrefix + "..."
	}
	return secretKeyPrefix + "..." + secretKey[len(secretKey)-4:]
}

// apiKeyCache memoises verified keys for a short window.
type apiKeyCache struct {
	mu      sync.RWMutex
	entries map[string]apiKeyCacheEntry
}

type apiKeyCacheEntry struct {
	projectID string
	keyID     string
	secretSum string
	expiresAt time.Time
}

func newAPIKeyCache() *apiKeyCache {
	return &apiKeyCache{entries: make(map[string]apiKeyCacheEntry)}
}

func (c *apiKeyCache) get(identifier string) (apiKeyCacheEntry, bool) {
	c.mu.RLock()
	entry, ok := c.entries[identifier]
	c.mu.RUnlock()

	if !ok || time.Now().After(entry.expiresAt) {
		return apiKeyCacheEntry{}, false
	}
	return entry, true
}

func (c *apiKeyCache) put(identifier string, entry apiKeyCacheEntry) {
	entry.expiresAt = time.Now().Add(apiKeyCacheTTL)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Bound the map so a flood of invalid identifiers cannot grow it without
	// limit; a full reset is cheap because entries are short-lived anyway.
	if len(c.entries) > 10000 {
		c.entries = make(map[string]apiKeyCacheEntry)
	}
	c.entries[identifier] = entry
}

// APIKeyMiddleware authenticates SDK requests and pins them to a project.
//
// Two schemes are accepted:
//
//	Authorization: Basic base64(publicKey:secretKey)   — what the Langfuse SDKs send
//	x-api-key: <key>                                    — this project's original scheme
//
// Both set the project id in the request context, so handlers below this point
// cannot see another project's data.
func APIKeyMiddleware(queries *db.Queries) func(http.Handler) http.Handler {
	cache := newAPIKeyCache()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			projectID, keyID, err := authenticateAPIKey(r, queries, cache)
			if err != nil {
				w.Header().Set("WWW-Authenticate", `Basic realm="obsevo"`)
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), ProjectIDKey, projectID)
			ctx = context.WithValue(ctx, APIKeyIDKey, keyID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// authenticateAPIKey resolves a request's credentials to a project.
func authenticateAPIKey(r *http.Request, queries *db.Queries, cache *apiKeyCache) (string, string, error) {
	if publicKey, secretKey, ok := r.BasicAuth(); ok {
		return authenticateKeyPair(r, queries, cache, publicKey, secretKey)
	}

	apiKey := r.Header.Get("x-api-key")
	if apiKey == "" {
		return "", "", fmt.Errorf("missing credentials: send HTTP Basic auth with your public and secret keys, or an x-api-key header")
	}
	return authenticateLegacyKey(r, queries, cache, apiKey)
}

// authenticateKeyPair verifies a Langfuse-style public/secret pair.
func authenticateKeyPair(r *http.Request, queries *db.Queries, cache *apiKeyCache, publicKey, secretKey string) (string, string, error) {
	if publicKey == "" || secretKey == "" {
		return "", "", fmt.Errorf("invalid API key")
	}

	digest := HashSecretKey(secretKey)

	if entry, ok := cache.get("pk:" + publicKey); ok {
		if subtle.ConstantTimeCompare([]byte(entry.secretSum), []byte(digest)) != 1 {
			return "", "", fmt.Errorf("invalid API key")
		}
		return entry.projectID, entry.keyID, nil
	}

	record, err := queries.GetAPIKeyByPublicKey(r.Context(), pgText(publicKey))
	if err != nil {
		return "", "", fmt.Errorf("invalid API key")
	}
	if !record.SecretKeyHash.Valid ||
		subtle.ConstantTimeCompare([]byte(record.SecretKeyHash.String), []byte(digest)) != 1 {
		return "", "", fmt.Errorf("invalid API key")
	}
	if record.ExpiresAt.Valid && time.Now().After(record.ExpiresAt.Time) {
		return "", "", fmt.Errorf("API key has expired")
	}

	cache.put("pk:"+publicKey, apiKeyCacheEntry{
		projectID: record.ProjectID,
		keyID:     record.ID,
		secretSum: record.SecretKeyHash.String,
	})
	// Best-effort usage stamp; a failure here must not reject the request.
	_ = queries.TouchAPIKey(r.Context(), record.ID)

	return record.ProjectID, record.ID, nil
}

// authenticateLegacyKey verifies a single-value x-api-key.
func authenticateLegacyKey(r *http.Request, queries *db.Queries, cache *apiKeyCache, apiKey string) (string, string, error) {
	if entry, ok := cache.get("legacy:" + apiKey); ok {
		return entry.projectID, entry.keyID, nil
	}

	record, err := queries.GetAPIKeyByKey(r.Context(), apiKey)
	if err != nil {
		return "", "", fmt.Errorf("invalid API key")
	}
	if record.ExpiresAt.Valid && time.Now().After(record.ExpiresAt.Time) {
		return "", "", fmt.Errorf("API key has expired")
	}

	cache.put("legacy:"+apiKey, apiKeyCacheEntry{
		projectID: record.ProjectID,
		keyID:     record.ID,
	})
	_ = queries.TouchAPIKey(r.Context(), record.ID)

	return record.ProjectID, record.ID, nil
}
