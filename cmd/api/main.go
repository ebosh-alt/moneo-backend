package main

import (
	"log"

	"moneo/backend/internal/bootstrap"
)

func main() {
	if _, err := bootstrap.NewAPI(bootstrap.Config{}); err != nil {
		log.Fatal(err)
	}
}
