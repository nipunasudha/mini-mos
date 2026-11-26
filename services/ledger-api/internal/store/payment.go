package store

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nipunasudha/mini-mos/services/ledger-api/internal/ledger"
)

type paymentResponse struct {
	PaymentID   uuid.UUID `json:"payment_id"`
	BillID      uuid.UUID `json:"bill_id"`
	Pot         string    `json:"pot"`
	AmountCents int64     `json:"amount_cents"`
}

// Pay pays a bill from its pot, once per idempotency key and once per bill.
// The request has no body, so the "request" we hash is the bill ID.
func (s *Store) Pay(ctx context.Context, billID uuid.UUID, key string) (Result, error) {
	return s.idempotent(ctx, "payment:"+billID.String(), key, Hash(billID.String()), func(tx pgx.Tx) (Result, error) {
		var clientID, pot, status string
		var amount int64
		err := tx.QueryRow(ctx,
			`SELECT client_id, pot, amount_cents, status FROM bills WHERE id = $1 FOR UPDATE`, billID).
			Scan(&clientID, &pot, &amount, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			return Result{}, reject(http.StatusNotFound, "bill_not_found", "no such bill")
		}
		if err != nil {
			return Result{}, err
		}
		if status == "paid" {
			// Returned as a normal Result, so the 409 IS stored and a retried key replays it.
			return Result{Status: http.StatusConflict, Err: "bill_already_paid",
				Body: errBody("bill_already_paid", "bill is already paid")}, nil
		}

		var balance int64
		err = tx.QueryRow(ctx, `SELECT balance_cents FROM accounts WHERE id = $1 FOR UPDATE`,
			ledger.AccountID(clientID, pot)).Scan(&balance)
		if err != nil {
			return Result{}, err
		}
		if balance < amount {
			// Rejection rolls back the key too, so a retry works once money arrives.
			body, _ := json.Marshal(map[string]any{
				"error": "insufficient_funds", "pot": pot, "needed_cents": amount, "available_cents": balance})
			return Result{}, &Rejection{http.StatusConflict, "insufficient_funds", body}
		}

		txnID, err := postTxn(ctx, tx, "payment", clientID, ledger.PaymentEntries(clientID, pot, amount))
		if err != nil {
			return Result{}, err
		}
		paymentID := uuid.New()
		_, err = tx.Exec(ctx, `INSERT INTO payments (id, bill_id, txn_id, amount_cents) VALUES ($1, $2, $3, $4)`,
			paymentID, billID, txnID, amount)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// Backstop: UNIQUE(bill_id). Unreachable while the bill row is locked,
			// but it must still answer correctly if someone ever removes that lock.
			return Result{}, reject(http.StatusConflict, "bill_already_paid", "bill is already paid")
		}
		if err != nil {
			return Result{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE bills SET status = 'paid' WHERE id = $1`, billID); err != nil {
			return Result{}, err
		}

		body, err := json.Marshal(paymentResponse{paymentID, billID, pot, amount})
		return Result{Status: http.StatusCreated, Body: body}, err
	})
}
