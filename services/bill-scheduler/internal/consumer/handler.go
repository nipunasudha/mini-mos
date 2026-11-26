package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nipunasudha/mini-mos/services/bill-scheduler/internal/ledgerclient"
	"github.com/nipunasudha/mini-mos/services/bill-scheduler/internal/metrics"
)

// errMalformed means the record itself is bad; retrying can't help.
var errMalformed = errors.New("malformed record")

// Handler pays the due bills for one deposits.allocated event. It knows
// nothing about Kafka: shortfalls go out through the publish function, so
// the logic can be tested with a fake ledger and a closure.
type Handler struct {
	Ledger      *ledgerclient.Client
	Metrics     *metrics.Metrics
	Publish     func(ctx context.Context, clientID string, ev Event) error
	HorizonDays int
	Now         func() time.Time
	RetryBase   time.Duration
}

// Handle returns nil only when every due bill is paid, already paid, or
// had its shortfall published. Any error means the record goes to the DLQ.
func (h *Handler) Handle(ctx context.Context, value []byte) error {
	clientID, depositID, err := parseDeposit(value)
	if err != nil {
		return fmt.Errorf("%w: %v", errMalformed, err)
	}

	dueBefore := h.Now().UTC().AddDate(0, 0, h.HorizonDays).Format(time.DateOnly)
	var bills []ledgerclient.Bill
	err = retry(ctx, h.RetryBase, func() (err error) {
		bills, err = h.Ledger.Bills(ctx, clientID, dueBefore)
		return err
	})
	if err != nil {
		return fmt.Errorf("list bills: %w", err)
	}

	for _, b := range bills {
		if err := h.payBill(ctx, clientID, depositID, b); err != nil {
			// Stop here: later bills would likely hit the same outage, and
			// the DLQ keeps the record so it can be replayed.
			return fmt.Errorf("bill %s: %w", b.BillID, err)
		}
	}
	return nil
}

func (h *Handler) payBill(ctx context.Context, clientID, depositID string, b ledgerclient.Bill) error {
	// Same key on every attempt and every redelivery: ledger-api pays once.
	key := "bill-" + b.BillID + "-" + b.DueDate
	var res ledgerclient.PayResult
	err := retry(ctx, h.RetryBase, func() (err error) {
		res, err = h.Ledger.Pay(ctx, b.BillID, key)
		return err
	})
	if err != nil {
		h.Metrics.PaymentAttempts.WithLabelValues("error").Inc()
		return err
	}
	h.Metrics.PaymentAttempts.WithLabelValues(res.Result).Inc()

	if res.Result != ledgerclient.InsufficientFunds {
		return nil
	}
	data, err := json.Marshal(Shortfall{
		BillID: b.BillID, Pot: b.Pot, Payee: b.Payee, DueDate: b.DueDate,
		NeededCents: res.NeededCents, AvailableCents: res.AvailableCents, DepositID: depositID,
	})
	if err != nil {
		return err
	}
	ev := Event{
		EventID:    shortfallID(b.BillID, b.DueDate, depositID),
		Type:       "pots.shortfall",
		OccurredAt: h.Now().UTC(),
		ClientID:   clientID,
		Data:       data,
	}
	if err := h.Publish(ctx, clientID, ev); err != nil {
		return fmt.Errorf("publish shortfall: %w", err)
	}
	h.Metrics.ShortfallsPublish.Inc()
	slog.Info("shortfall published", "client_id", clientID, "bill_id", b.BillID, "pot", b.Pot)
	return nil
}
