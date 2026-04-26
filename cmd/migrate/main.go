package main

import (
	"context"
	"fmt"
	"os"

	"moneo/internal/bootstrap"
)

func main() {
	ctx := context.Background()
	migrator := bootstrap.NewMigrator()
	if err := migrator.Run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
