package store

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/nipunasudha/mini-mos/services/ledger-api/internal/ledger"
	"github.com/nipunasudha/mini-mos/services/ledger-api/internal/outbox"
)

var errUnbalanced = errors.New("ledger entries do not sum to zero")

type DepositRequest struct {
	AmountCents int64  `json:"amount_cents"`
	Source      string `json:"source"`
}

// depositEvent is the `data` of the deposits.allocated event (spec Section 6).
type depositEvent struct {
	DepositID   uuid.UUID      `json:"deposit_id"`
	AmountCents int64          `json:"amount_cents"`
	Allocations []ledger.Share `json:"allocations"`
}

type depositResponse struct {
	DepositID   uuid.UUID      `json:"deposit_id"`
	ClientID    string         `json:"client_id"`
	AmountCents int64          `json:"amount_cents"`
	Allocations []ledger.Share `json:"allocations"`
}

// Deposit splits the amount over the client's pots, once per idempotency key.
func (s *Store) Deposit(ctx context.Context, clientID, key string, req DepositRequest) (Result, error) {
	return s.idempotent(ctx, "deposit:"+clientID, key, Hash(req), func(tx pgx.Tx) (Result, error) {
		// Lock all of the client's pots in id order. A fixed order means two
		// concurrent deposits for one client queue up instead of deadlocking.
		rows, err := tx.Query(ctx, `SELECT id FROM accounts WHERE client_id = $1 ORDER BY id FOR UPDATE`, clientID)
		if err != nil {
			return Result{}, err
		}
		pots, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return Result{}, err
		}
		if len(pots) == 0 {
			return Result{}, reject(http.StatusNotFound, "client_not_found", "no such client")
		}

		rows, err = tx.Query(ctx, `SELECT pot, amount_cents, priority FROM plan_rules WHERE client_id = $1`, clientID)
		if err != nil {
			return Result{}, err
		}
		rules, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ledger.Rule, error) {
			var x ledger.Rule
			return x, r.Scan(&x.Pot, &x.AmountCents, &x.Priority)
		})
		if err != nil {
			return Result{}, err
		}

		shares := ledger.Allocate(req.AmountCents, rules)
		txnID, err := postTxn(ctx, tx, "deposit", clientID, ledger.DepositEntries(clientID, req.AmountCents, shares))
		if err != nil {
			return Result{}, err
		}
		depositID := uuid.New()
		if _, err := tx.Exec(ctx,
			`INSERT INTO deposits (id, client_id, amount_cents, source, txn_id) VALUES ($1, $2, $3, $4, $5)`,
			depositID, clientID, req.AmountCents, req.Source, txnID); err != nil {
			return Result{}, err
		}
		event := depositEvent{depositID, req.AmountCents, shares}
		if err := outbox.Insert(ctx, tx, "deposits.allocated", clientID, event); err != nil {
			return Result{}, err
		}

		body, err := json.Marshal(depositResponse{depositID, clientID, req.AmountCents, shares})
		return Result{Status: http.StatusCreated, Body: body}, err
	})
}
