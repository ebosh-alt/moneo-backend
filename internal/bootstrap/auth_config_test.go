package bootstrap

import (
	"testing"
	"time"
)

func TestLoadAuthTokenConfigFromEnvDefaults(t *testing.T) {
	t.Setenv("AUTH_JWT_SECRET", "super-secret")
	t.Setenv("AUTH_ACCESS_TOKEN_TTL", "")
	t.Setenv("AUTH_REFRESH_TOKEN_TTL", "")

	cfg, err := LoadAuthTokenConfigFromEnv()
	if err != nil {
		t.Fatalf("load auth token config: %v", err)
	}

	if cfg.AccessTokenTTL != 15*time.Minute {
		t.Fatalf("expected default access ttl 15m, got %s", cfg.AccessTokenTTL)
	}
	if cfg.RefreshTokenTTL != 30*24*time.Hour {
		t.Fatalf("expected default refresh ttl 30d, got %s", cfg.RefreshTokenTTL)
	}
	if cfg.JWTSecret != "super-secret" {
		t.Fatalf("expected jwt secret from env, got %q", cfg.JWTSecret)
	}
}

func TestLoadAuthTokenConfigFromEnvCustomDurations(t *testing.T) {
	t.Setenv("AUTH_JWT_SECRET", "super-secret")
	t.Setenv("AUTH_ACCESS_TOKEN_TTL", "10m")
	t.Setenv("AUTH_REFRESH_TOKEN_TTL", "720h")

	cfg, err := LoadAuthTokenConfigFromEnv()
	if err != nil {
		t.Fatalf("load auth token config: %v", err)
	}

	if cfg.AccessTokenTTL != 10*time.Minute {
		t.Fatalf("expected access ttl 10m, got %s", cfg.AccessTokenTTL)
	}
	if cfg.RefreshTokenTTL != 30*24*time.Hour {
		t.Fatalf("expected refresh ttl 720h, got %s", cfg.RefreshTokenTTL)
	}
}
