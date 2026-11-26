package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nipunasudha/mini-mos/services/ledger-api/internal/metrics"
	"github.com/twmb/franz-go/pkg/kgo"
)

type Relay struct {
	Pool      *pgxpool.Pool
	Kafka     *kgo.Client // must be created with kgo.RequiredAcks(kgo.AllISRAcks())
	Metrics   *metrics.Metrics
	Interval  time.Duration
	BatchSize int
}

// Run ticks until ctx is cancelled. A tick that has started always finishes
// (it uses its own context), so shutdown never abandons a half-published batch.
func (r *Relay) Run(ctx context.Context) {
	t := time.NewTicker(r.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		tickCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		if err := r.tick(tickCtx); err != nil {
			// Not fatal: the rows stay unpublished and the next tick retries.
			slog.Error("outbox relay tick failed", "err", err)
		}
		r.refreshPending(tickCtx)
		cancel()
	}
}

// tick publishes one batch. Rows are locked with SKIP LOCKED so several
// replicas can relay in parallel without taking the same row twice.
func (r *Relay) tick(ctx context.Context) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // no-op once committed

	rows, err := tx.Query(ctx,
		`SELECT id, event_id, topic, msg_key, payload FROM outbox
		 WHERE published_at IS NULL ORDER BY id LIMIT $1 FOR UPDATE SKIP LOCKED`, r.BatchSize)
	if err != nil {
		return err
	}
	var ids []int64
	var recs []*kgo.Record
	for rows.Next() {
		var (
			id         int64
			eventID    string
			topic, key string
			payload    []byte
		)
		if err := rows.Scan(&id, &eventID, &topic, &key, &payload); err != nil {
			return err
		}
		ids = append(ids, id)
		recs = append(recs, &kgo.Record{
			Topic:   topic,
			Key:     []byte(key),
			Value:   payload,
			Headers: []kgo.RecordHeader{{Key: "event_id", Value: []byte(eventID)}},
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(recs) == 0 {
		return nil
	}

	// Any failure rolls the whole batch back (deferred Rollback): rows already
	// accepted by Kafka get sent again next tick. At-least-once; consumers dedupe.
	for _, res := range r.Kafka.ProduceSync(ctx, recs...) {
		if res.Err != nil {
			return fmt.Errorf("produce to %s: %w", res.Record.Topic, res.Err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE outbox SET published_at = now() WHERE id = ANY($1)`, ids); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	r.Metrics.OutboxPublished.Add(float64(len(ids)))
	return nil
}

func (r *Relay) refreshPending(ctx context.Context) {
	var n int64
	if err := r.Pool.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE published_at IS NULL`).Scan(&n); err != nil {
		slog.Error("outbox pending count failed", "err", err)
		return
	}
	r.Metrics.OutboxPending.Set(float64(n))
}
