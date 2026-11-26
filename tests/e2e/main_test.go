//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

var (
	baseURL      string
	redeliverCmd string        // empty means: skip the redelivery test
	timeout      time.Duration // per "eventually" check
)

// httpc has its own timeout so a hung server fails a test instead of hanging the run.
var httpc = &http.Client{Timeout: 10 * time.Second}

func TestMain(m *testing.M) {
	baseURL = strings.TrimRight(os.Getenv("BASE_URL"), "/")
	if baseURL == "" {
		fmt.Fprintln(os.Stderr, "BASE_URL is required, e.g. BASE_URL=http://localhost:8080")
		os.Exit(1)
	}
	redeliverCmd = os.Getenv("REDELIVER_CMD")

	timeout = 30 * time.Second
	if s := os.Getenv("E2E_TIMEOUT"); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bad E2E_TIMEOUT:", err)
			os.Exit(1)
		}
		timeout = d
	}

	if err := waitReady(60 * time.Second); err != nil {
		fmt.Fprintln(os.Stderr, "stack not ready:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// waitReady polls /readyz so the suite can start right after `docker compose up`.
func waitReady(limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for {
		resp, err := httpc.Get(baseURL + "/readyz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			err = fmt.Errorf("status %d", resp.StatusCode)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s/readyz not OK after %s: %w", baseURL, limit, err)
		}
		<-time.After(time.Second)
	}
}

// eventually polls fn until it returns nil. It exists because the scheduler pays
// bills asynchronously (outbox -> Kafka -> scheduler -> API), so results appear
// "soon", not instantly. Never use a fixed sleep for that.
func eventually(t *testing.T, limit, interval time.Duration, fn func() error) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for {
		err := fn()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("not true after %s: %v", limit, err)
		}
		<-time.After(interval)
	}
}
