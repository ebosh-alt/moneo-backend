package main

import (
	"log"

	"moneo/internal/bootstrap"
)

func main() {
	if _, err := bootstrap.NewAPI(bootstrap.Config{}); err != nil {
		log.Fatal(err)
	}
}
