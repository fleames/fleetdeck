package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/api"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/config"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/db"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/realtime"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/relay"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/worker"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	hub := realtime.NewHub()
	srv := api.New(cfg, pool, hub)
	worker.New(pool, hub, cfg.AgentOfflineAfter, cfg.RawRetentionDays, cfg.Agg5mRetentionDays, cfg.Agg1hRetentionDays).Start(ctx)

	httpServer := &http.Server{
		Addr:              cfg.APIAddr,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("FleetDeck API listening on %s", cfg.APIAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	if cfg.RelayURL != "" && cfg.RelayToken != "" {
		edge := &relay.Client{
			RelayURL:  cfg.RelayURL,
			Token:     cfg.RelayToken,
			LocalURL:  cfg.RelayLocalURL,
		}
		go edge.Run(ctx)
		log.Printf("FleetDeck edge relay enabled → %s (agents use %s)", cfg.RelayURL, cfg.APIPublicURL)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = httpServer.Shutdown(shutdownCtx)
}
