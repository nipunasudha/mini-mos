package consumer

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/nipunasudha/mini-mos/services/bill-scheduler/internal/ledgerclient"
)

func TestBackoffBounds(t *testing.T) {
	base := 200 * time.Millisecond
	tests := []struct {
		attempt  int
		min, max time.Duration
	}{
		{1, 100 * time.Millisecond, 200 * time.Millisecond},
		{2, 200 * time.Millisecond, 400 * time.Millisecond},
		{3, 400 * time.Millisecond, 800 * time.Millisecond},
		{4, 800 * time.Millisecond, 1600 * time.Millisecond},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprint("attempt", tc.attempt), func(t *testing.T) {
			for range 1000 {
				if d := backoff(tc.attempt, base); d < tc.min || d > tc.max {
					t.Fatalf("backoff = %v, want within [%v, %v]", d, tc.min, tc.max)
				}
			}
		})
	}
}

func TestRetry(t *testing.T) {
	transient := fmt.Errorf("%w: boom", ledgerclient.ErrTransient)
	final := errors.New("bad request")
	tests := []struct {
		name      string
		failFirst int // calls that fail with err before succeeding
		err       error
		wantCalls int
		wantErr   bool
	}{
		{"succeeds first time", 0, transient, 1, false},
		{"transient then ok", 2, transient, 3, false},
		{"transient forever stops at max attempts", 100, transient, maxAttempts, true},
		{"non-transient is not retried", 100, final, 1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			err := retry(context.Background(), time.Microsecond, func() error {
				calls++
				if calls <= tc.failFirst {
					return tc.err
				}
				return nil
			})
			if calls != tc.wantCalls || (err != nil) != tc.wantErr {
				t.Fatalf("calls=%d err=%v, want calls=%d wantErr=%v", calls, err, tc.wantCalls, tc.wantErr)
			}
		})
	}
}

func TestRetryStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	err := retry(ctx, time.Hour, func() error {
		calls++
		return ledgerclient.ErrTransient
	})
	if calls != 1 || !errors.Is(err, context.Canceled) {
		t.Fatalf("calls=%d err=%v, want 1 call and context.Canceled", calls, err)
	}
}
