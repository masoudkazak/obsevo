package config_test

import (
	"testing"

	"github.com/obsevo/obsevo/internal/config"
)

func TestWarnDefaults_DefaultSecrets(t *testing.T) {
	cfg := &config.Config{
		AppSecret:     "change-me-in-production",
		JWTSecret:     "change-me-in-production",
		RedisPassword: "",
	}

	// WarnDefaults should not panic; it only logs.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("WarnDefaults panicked: %v", r)
		}
	}()

	cfg.WarnDefaults()
}

func TestWarnDefaults_CustomSecrets(t *testing.T) {
	cfg := &config.Config{
		AppSecret:     "a-very-long-secret-key-that-is-definitely-not-default",
		JWTSecret:     "another-very-long-secret-key-that-is-definitely-not-default",
		RedisPassword: "some-password",
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("WarnDefaults panicked: %v", r)
		}
	}()

	cfg.WarnDefaults()
}

func TestLoadDefaults(t *testing.T) {
	cfg := config.Load()

	if cfg.AppPort != 3001 {
		t.Errorf("expected default AppPort 3001, got %d", cfg.AppPort)
	}
	if cfg.MaxBodyBytes != 10<<20 {
		t.Errorf("expected default MaxBodyBytes %d, got %d", int64(10<<20), cfg.MaxBodyBytes)
	}
	if cfg.DBMaxConns != 20 {
		t.Errorf("expected default DBMaxConns 20, got %d", cfg.DBMaxConns)
	}
}
