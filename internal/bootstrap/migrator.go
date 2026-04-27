package bootstrap

import (
	"context"

	opsmigrate "moneo/internal/ops/migrate"
)

type Migrator struct {
	runner *opsmigrate.Runner
}

func NewMigrator() *Migrator {
	return &Migrator{runner: opsmigrate.NewRunner()}
}

func (m *Migrator) Run(ctx context.Context, args []string) error {
	return m.runner.Run(ctx, args)
}
