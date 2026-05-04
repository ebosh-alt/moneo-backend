package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"

	opsrepair "moneo/internal/ops/repair"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Ops struct {
	pool         *pgxpool.Pool
	repairRunner *opsrepair.Runner
}

func NewOps() (*Ops, error) {
	postgresCfg, err := LoadPostgresConfigFromEnv()
	if err != nil {
		return nil, fmt.Errorf("load postgres config: %w", err)
	}

	pool, err := pgxpool.New(context.Background(), postgresCfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open postgres pool: %w", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &Ops{
		pool:         pool,
		repairRunner: opsrepair.NewRunner(pool, os.Stdout),
	}, nil
}

func (o *Ops) Run(ctx context.Context, args []string) error {
	if o == nil || o.pool == nil {
		return errors.New("ops runtime is not initialized")
	}
	defer o.pool.Close()

	if len(args) == 0 {
		return o.usageError()
	}

	switch args[0] {
	case "repair":
		return o.repairRunner.Run(ctx, args[1:])
	default:
		return o.usageError()
	}
}

func (o *Ops) usageError() error {
	return errors.New("usage: go run ./cmd/ops <command> [args]\ncommands: repair")
}
