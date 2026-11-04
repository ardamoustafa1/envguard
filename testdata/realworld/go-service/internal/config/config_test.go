package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-only-secret")
	t.Setenv("DB_URL", "postgres://app:app@localhost:5432/app")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", cfg.HTTP.Addr)
	}
	if cfg.HTTP.ReadTimeout != 5*time.Second {
		t.Errorf("ReadTimeout = %v", cfg.HTTP.ReadTimeout)
	}
	if cfg.DB.MaxConns != 10 {
		t.Errorf("MaxConns = %d", cfg.DB.MaxConns)
	}
}

func TestLoadRequiresDBURL(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-only-secret")
	t.Setenv("DB_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected error for empty DB_URL")
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-only-secret")
	t.Setenv("DB_URL", "postgres://app:app@localhost:5432/app")
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("REDIS_TTL", "30s")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Addr != ":9090" || cfg.Redis.TTL != 30*time.Second {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}
