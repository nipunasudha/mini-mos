// ledger-api: subcommands `serve` (default), `migrate` and `seed`.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nipunasudha/mini-mos/services/ledger-api/internal/api"
	"github.com/nipunasudha/mini-mos/services/ledger-api/internal/config"
	"github.com/nipunasudha/mini-mos/services/ledger-api/internal/metrics"
	"github.com/nipunasudha/mini-mos/services/ledger-api/internal/outbox"
	"github.com/nipunasudha/mini-mos/services/ledger-api/internal/store"
	"github.com/twmb/franz-go/pkg/kgo"
)

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	cfg, err := config.Load()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})))
	if err == nil {
		err = run(cmd, cfg)
	}
	if err != nil {
		slog.Error("ledger-api failed", "cmd", cmd, "err", err)
		os.Exit(1)
	}
}

func run(cmd string, cfg config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	switch cmd {
	case "migrate":
		return store.Migrate(ctx, cfg.DatabaseURL)
	case "seed":
		return seed(ctx, cfg)
	case "serve":
		return serve(ctx, cfg)
	default:
		return fmt.Errorf("unknown subcommand %q (want serve, migrate or seed)", cmd)
	}
}

func seed(ctx context.Context, cfg config.Config) error {
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	created, err := store.New(pool).SeedClient(ctx, "c-1001", "Demo Client")
	slog.Info("seed done", "client_id", "c-1001", "created", created)
	return err
}

func serve(ctx context.Context, cfg config.Config) error {
	if len(cfg.KafkaBrokers) == 0 {
		return errors.New("KAFKA_BROKERS is required")
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	// acks=all: a record counts as published only once every in-sync replica has it.
	kc, err := kgo.NewClient(kgo.SeedBrokers(cfg.KafkaBrokers...), kgo.RequiredAcks(kgo.AllISRAcks()))
	if err != nil {
		return err
	}

	m := metrics.New()
	apiServer := api.New(store.New(pool), m, cfg.AdminAPIEnabled)
	httpServer := &http.Server{Addr: ":" + cfg.Port, Handler: apiServer.Handler(), ReadHeaderTimeout: 5 * time.Second}

	relay := &outbox.Relay{Pool: pool, Kafka: kc, Metrics: m, Interval: cfg.OutboxPollInterval, BatchSize: cfg.OutboxBatchSize}
	relayCtx, stopRelay := context.WithCancel(context.Background()) // separate from ctx: stopped in order, below
	relayDone := make(chan struct{})
	go func() { relay.Run(relayCtx); close(relayDone) }()

	serveErr := make(chan error, 1)
	go func() { serveErr <- httpServer.ListenAndServe() }()
	slog.Info("ledger-api listening", "port", cfg.Port, "admin_api", cfg.AdminAPIEnabled)

	select {
	case err := <-serveErr: // could not start (e.g. port in use)
		stopRelay()
		return err
	case <-ctx.Done():
	}

	// Shutdown order, all inside one SHUTDOWN_TIMEOUT budget:
	// 1. readiness fails  2. HTTP drains  3. relay finishes its batch  4. close pool and Kafka.
	slog.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	apiServer.StartShutdown()
	err = httpServer.Shutdown(shutCtx)
	stopRelay()
	select {
	case <-relayDone:
	case <-shutCtx.Done():
		slog.Warn("relay did not stop in time")
	}
	closed := make(chan struct{})
	go func() { pool.Close(); kc.Close(); close(closed) }()
	select {
	case <-closed:
	case <-shutCtx.Done():
		slog.Warn("pool or Kafka client did not close in time")
	}
	return err
}
