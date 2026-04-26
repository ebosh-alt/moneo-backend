package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"moneo/internal/bootstrap"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	api, err := bootstrap.NewAPI(bootstrap.Config{})
	if err != nil {
		log.Fatal(err)
	}

	if err := api.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
