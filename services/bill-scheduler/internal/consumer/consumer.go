// Package consumer wires the Kafka group to the Handler.
package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/nipunasudha/mini-mos/services/bill-scheduler/internal/config"
	"github.com/nipunasudha/mini-mos/services/bill-scheduler/internal/ledgerclient"
	"github.com/nipunasudha/mini-mos/services/bill-scheduler/internal/metrics"
)

const (
	topicIn        = "deposits.allocated"
	topicDLQ       = "deposits.allocated.dlq"
	topicShortfall = "pots.shortfall"
)

type Consumer struct {
	client  *kgo.Client
	handler *Handler
	metrics *metrics.Metrics
	ready   atomic.Bool
}

func New(cfg config.Config, m *metrics.Metrics) (*Consumer, error) {
	c := &Consumer{metrics: m}

	// With a manual commit after every record, a rebalance can't lose
	// uncommitted work, so the callbacks only have to track readiness.
	setReady := func(v bool) func(context.Context, *kgo.Client, map[string][]int32) {
		return func(context.Context, *kgo.Client, map[string][]int32) { c.ready.Store(v) }
	}
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ConsumerGroup(cfg.Group),
		kgo.ConsumeTopics(topicIn),
		// A brand-new group must see events produced before it started.
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.DisableAutoCommit(),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.OnPartitionsAssigned(setReady(true)),
		kgo.OnPartitionsRevoked(setReady(false)),
		kgo.OnPartitionsLost(setReady(false)),
	)
	if err != nil {
		return nil, err
	}
	c.client = cl
	c.handler = &Handler{
		Ledger:      ledgerclient.New(cfg.LedgerURL),
		Metrics:     m,
		Publish:     c.publishShortfall,
		HorizonDays: cfg.DueHorizonDays,
		Now:         time.Now,
		RetryBase:   baseDelay,
	}
	return c, nil
}

// Ready is true once the group has been joined and partitions assigned.
func (c *Consumer) Ready() bool { return c.ready.Load() }

// Run handles records one at a time until ctx is cancelled. Cancelling
// only stops the polling: a record already being handled is finished and
// committed first. It returns an error if a record could neither be
// handled nor parked in the DLQ; the offset stays uncommitted, so the
// record is redelivered after a restart.
func (c *Consumer) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		fetches := c.client.PollRecords(ctx, 1)
		if ctx.Err() != nil {
			break
		}
		if fetches.IsClientClosed() {
			return errors.New("kafka client closed")
		}
		fetches.EachError(func(t string, p int32, err error) {
			slog.Error("fetch failed", "topic", t, "partition", p, "error", err)
		})
		if fetches.NumRecords() == 0 {
			// Don't spin on a persistent fetch error.
			if fetches.Err0() != nil {
				select {
				case <-time.After(time.Second):
				case <-ctx.Done():
				}
			}
			continue
		}

		for _, r := range fetches.Records() {
			if err := c.process(ctx, r); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Consumer) process(ctx context.Context, r *kgo.Record) error {
	// Detached from ctx so that SIGTERM doesn't abort a half-paid record.
	// The shutdown timeout in main still bounds the wait.
	hctx := context.WithoutCancel(ctx)

	result := "ok"
	if err := c.handler.Handle(hctx, r.Value); err != nil {
		slog.Error("sending record to DLQ", "partition", r.Partition, "offset", r.Offset, "error", err)
		dlq := &kgo.Record{
			Topic:   topicDLQ,
			Key:     r.Key,
			Value:   r.Value,
			Headers: []kgo.RecordHeader{{Key: "error", Value: []byte(err.Error())}},
		}
		if err := c.client.ProduceSync(hctx, dlq).FirstErr(); err != nil {
			return fmt.Errorf("produce to DLQ: %w", err)
		}
		result = "dlq"
	}
	c.metrics.Events.WithLabelValues(result).Inc()

	// Only now is the record fully handled; committing earlier would lose
	// it if we crashed mid-way.
	if err := c.client.CommitRecords(hctx, r); err != nil {
		return fmt.Errorf("commit offset: %w", err)
	}
	return nil
}

func (c *Consumer) publishShortfall(ctx context.Context, clientID string, ev Event) error {
	value, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	rec := &kgo.Record{
		Topic:   topicShortfall,
		Key:     []byte(clientID),
		Value:   value,
		Headers: []kgo.RecordHeader{{Key: "event_id", Value: []byte(ev.EventID)}},
	}
	return c.client.ProduceSync(ctx, rec).FirstErr()
}

// Close leaves the group explicitly, so a stale member can't block a later
// offset reset or delay the next rebalance, then closes the client.
func (c *Consumer) Close(ctx context.Context) {
	c.ready.Store(false)
	if err := c.client.LeaveGroupContext(ctx); err != nil {
		slog.Warn("leave group", "error", err)
	}
	c.client.Close()
}
