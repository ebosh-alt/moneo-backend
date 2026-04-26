package bootstrap

import (
	"strings"
	"testing"
)

func TestLoadPostgresConfigFromEnvUsesPostgresURL(t *testing.T) {
	t.Setenv("POSTGRES_URL", "postgres://user:pass@localhost:5432/app?sslmode=disable")
	t.Setenv("POSTGRES_HOST", "")
	t.Setenv("POSTGRES_USER", "")
	t.Setenv("POSTGRES_PASSWORD", "")
	t.Setenv("POSTGRES_DBNAME", "")

	cfg, err := LoadPostgresConfigFromEnv()
	if err != nil {
		t.Fatalf("load postgres config: %v", err)
	}

	if cfg.DSN != "postgres://user:pass@localhost:5432/app?sslmode=disable" {
		t.Fatalf("unexpected dsn: %q", cfg.DSN)
	}
}

func TestLoadPostgresConfigFromEnvBuildsDSNFromParts(t *testing.T) {
	t.Setenv("POSTGRES_URL", "")
	t.Setenv("POSTGRES_HOST", "db")
	t.Setenv("POSTGRES_PORT", "5433")
	t.Setenv("POSTGRES_USER", "video")
	t.Setenv("POSTGRES_PASSWORD", "s3cr3t")
	t.Setenv("POSTGRES_DBNAME", "accounts")
	t.Setenv("POSTGRES_SSLMODE", "disable")

	cfg, err := LoadPostgresConfigFromEnv()
	if err != nil {
		t.Fatalf("load postgres config: %v", err)
	}

	if !strings.Contains(cfg.DSN, "postgres://video:s3cr3t@db:5433/accounts") {
		t.Fatalf("dsn must contain expected base, got %q", cfg.DSN)
	}
	if !strings.Contains(cfg.DSN, "sslmode=disable") {
		t.Fatalf("dsn must include sslmode, got %q", cfg.DSN)
	}
}

func TestLoadPostgresConfigFromEnvRequiresPartsWhenURLMissing(t *testing.T) {
	t.Setenv("POSTGRES_URL", "")
	t.Setenv("POSTGRES_HOST", "")
	t.Setenv("POSTGRES_USER", "video")
	t.Setenv("POSTGRES_PASSWORD", "video")
	t.Setenv("POSTGRES_DBNAME", "accounts")

	_, err := LoadPostgresConfigFromEnv()
	if err == nil {
		t.Fatal("expected error when POSTGRES_HOST is missing")
	}
}
