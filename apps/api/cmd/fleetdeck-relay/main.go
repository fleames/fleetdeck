package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/relay"
)

func main() {
	addr := getenv("RELAY_ADDR", ":8090")
	token := os.Getenv("RELAY_TOKEN")
	if token == "" {
		log.Fatal("RELAY_TOKEN is required")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := relay.Run(ctx, addr, token); err != nil {
		log.Fatal(err)
	}
}

func getenv(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
