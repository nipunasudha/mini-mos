// Package config reads the service settings from environment variables.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port            string
	Brokers         []string
	Group           string
	LedgerURL       string
	DueHorizonDays  int
	LogLevel        slog.Level
	ShutdownTimeout time.Duration
}

// Load fails fast on a missing or malformed value, so a bad deploy
// crash-loops visibly instead of running with a silent default.
func Load() (Config, error) {
	c := Config{
		Port:      get("PORT", "8080"),
		Group:     get("KAFKA_GROUP", "bill-scheduler"),
		LedgerURL: strings.TrimRight(get("LEDGER_URL", "http://ledger-api.mos.svc:8080"), "/"),
	}

	for _, b := range strings.Split(os.Getenv("KAFKA_BROKERS"), ",") {
		if b = strings.TrimSpace(b); b != "" {
			c.Brokers = append(c.Brokers, b)
		}
	}
	if len(c.Brokers) == 0 {
		return c, fmt.Errorf("KAFKA_BROKERS is required")
	}

	var err error
	if c.DueHorizonDays, err = strconv.Atoi(get("DUE_HORIZON_DAYS", "7")); err != nil || c.DueHorizonDays < 0 {
		return c, fmt.Errorf("DUE_HORIZON_DAYS must be a non-negative integer")
	}
	if c.ShutdownTimeout, err = time.ParseDuration(get("SHUTDOWN_TIMEOUT", "15s")); err != nil {
		return c, fmt.Errorf("SHUTDOWN_TIMEOUT: %w", err)
	}
	if err = c.LogLevel.UnmarshalText([]byte(get("LOG_LEVEL", "info"))); err != nil {
		return c, fmt.Errorf("LOG_LEVEL: %w", err)
	}
	return c, nil
}

func get(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
