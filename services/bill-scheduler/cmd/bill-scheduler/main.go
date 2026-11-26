package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/nipunasudha/mini-mos/services/bill-scheduler/internal/config"
	"github.com/nipunasudha/mini-mos/services/bill-scheduler/internal/consumer"
	"github.com/nipunasudha/mini-mos/services/bill-scheduler/internal/metrics"
)

func main() {
	if err := run(); err != nil {
		slog.Error("bill-scheduler failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})))

	m := metrics.New()
	c, err := consumer.New(cfg, m)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !c.Ready() {
			http.Error(w, "not joined the consumer group", http.StatusServiceUnavailable)
		}
	})
	mux.Handle("/metrics", promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{}))
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	httpErr := make(chan error, 1)
	go func() { httpErr <- srv.ListenAndServe() }()
	runErr := make(chan error, 1)
	go func() { runErr <- c.Run(ctx) }()

	// Wait for a signal, or for either side to die on its own.
	select {
	case err = <-runErr:
	case err = <-httpErr:
	case <-ctx.Done():
		slog.Info("shutting down")
		// Run returns once the current record is handled and committed.
		select {
		case err = <-runErr:
		case <-time.After(cfg.ShutdownTimeout):
			err = errors.New("timed out waiting for the current record")
		}
	}
	stop()

	// Fresh context: ctx is already cancelled by now.
	sctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	c.Close(sctx)
	if serr := srv.Shutdown(sctx); serr != nil {
		slog.Warn("http shutdown", "error", serr)
	}
	return err
}
