package main

import (
	"context"
	"fmt"
	"os"

	"moneo/internal/bootstrap"
)

func main() {
	ctx := context.Background()

	ops, err := bootstrap.NewOps()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := ops.Run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
