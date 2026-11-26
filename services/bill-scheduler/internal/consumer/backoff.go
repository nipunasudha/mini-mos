package consumer

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/nipunasudha/mini-mos/services/bill-scheduler/internal/ledgerclient"
)

const (
	maxAttempts = 5
	baseDelay   = 200 * time.Millisecond
)

// backoff returns the wait after the given failed attempt (1-based):
// base*2^(attempt-1), then jittered into the upper half of that range so
// that many failing consumers don't retry in lockstep.
func backoff(attempt int, base time.Duration) time.Duration {
	d := base << (attempt - 1)
	return d/2 + rand.N(d/2+1)
}

// retry runs fn up to maxAttempts times, sleeping between attempts, but only
// for transient errors. A final error is returned as is.
func retry(ctx context.Context, base time.Duration, fn func() error) error {
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err = fn(); err == nil || !errors.Is(err, ledgerclient.ErrTransient) {
			return err
		}
		if attempt == maxAttempts {
			break
		}
		select {
		case <-time.After(backoff(attempt, base)):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}
