package migrate

import (
	"context"
	"errors"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

const (
	DefaultMigrationsDir = "db/migrations"
	EnvPostgresURL       = "POSTGRES_URL"
	EnvMigrationsDir     = "MIGRATIONS_DIR"
)

type Runner struct {
	defaultMigrationsDir string
}

func NewRunner() *Runner {
	return &Runner{defaultMigrationsDir: DefaultMigrationsDir}
}

func (r *Runner) Run(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return r.UsageError()
	}

	command := args[0]
	migrationsDir := os.Getenv(EnvMigrationsDir)
	if migrationsDir == "" {
		migrationsDir = r.defaultMigrationsDir
	}

	switch command {
	case "create":
		if len(args) < 2 {
			return errors.New("migration name is required: go run ./cmd/migrate create <name>")
		}
		if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
			return fmt.Errorf("create migrations directory: %w", err)
		}
		if err := goose.Create(nil, migrationsDir, args[1], "sql"); err != nil {
			return fmt.Errorf("create migration: %w", err)
		}
		return nil
	case "fix":
		if err := goose.Fix(migrationsDir); err != nil {
			return fmt.Errorf("fix migrations: %w", err)
		}
		return nil
	}

	postgresURL := os.Getenv(EnvPostgresURL)
	if postgresURL == "" {
		return fmt.Errorf("%s is required for %q command", EnvPostgresURL, command)
	}

	db, err := goose.OpenDBWithDriver("postgres", postgresURL)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	if err := goose.RunContext(ctx, command, db, migrationsDir, args[1:]...); err != nil {
		return fmt.Errorf("run goose command %q: %w", command, err)
	}
	return nil
}

func (r *Runner) UsageError() error {
	return errors.New("usage: go run ./cmd/migrate <command> [args]\ncommands: create <name>, up, down, status, version, fix")
}
