// Package config reads all settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port               string
	DatabaseURL        string
	KafkaBrokers       []string
	OutboxPollInterval time.Duration
	OutboxBatchSize    int
	ShutdownTimeout    time.Duration
	LogLevel           slog.Level
	AdminAPIEnabled    bool
}

// Load returns every setting. Only DATABASE_URL is required here; KAFKA_BROKERS
// is checked by `serve`, because `migrate` and `seed` never talk to Kafka.
func Load() (Config, error) {
	var errs []error
	get := func(name, def string) string {
		if v := os.Getenv(name); v != "" {
			return v
		}
		return def
	}
	duration := func(name, def string) time.Duration {
		d, err := time.ParseDuration(get(name, def))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
		return d
	}
	integer := func(name, def string) int {
		n, err := strconv.Atoi(get(name, def))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
		return n
	}

	c := Config{
		Port:               get("PORT", "8080"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		OutboxPollInterval: duration("OUTBOX_POLL_INTERVAL", "500ms"),
		OutboxBatchSize:    integer("OUTBOX_BATCH_SIZE", "100"),
		ShutdownTimeout:    duration("SHUTDOWN_TIMEOUT", "15s"),
		AdminAPIEnabled:    get("ADMIN_API_ENABLED", "false") == "true",
	}
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	for _, b := range strings.Split(os.Getenv("KAFKA_BROKERS"), ",") {
		if b = strings.TrimSpace(b); b != "" {
			c.KafkaBrokers = append(c.KafkaBrokers, b)
		}
	}
	if err := c.LogLevel.UnmarshalText([]byte(get("LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL: %w", err))
	}
	return c, errors.Join(errs...)
}
