package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/nipunasudha/mini-mos/services/ledger-api/internal/ledger"
)

// postTxn writes one ledger transaction and moves the pot balances. It is the
// single place where money moves, for deposits and payments alike.
func postTxn(ctx context.Context, tx pgx.Tx, kind, clientID string, entries []ledger.Entry) (uuid.UUID, error) {
	// Refuse to write an unbalanced transaction, whatever the caller did.
	if ledger.Sum(entries) != 0 {
		return uuid.Nil, errUnbalanced
	}
	txnID := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO ledger_txns (id, kind, client_id) VALUES ($1, $2, $3)`, txnID, kind, clientID); err != nil {
		return uuid.Nil, err
	}
	for _, e := range entries {
		if _, err := tx.Exec(ctx, `INSERT INTO ledger_entries (txn_id, account_id, amount_cents) VALUES ($1, $2, $3)`,
			txnID, e.AccountID, e.AmountCents); err != nil {
			return uuid.Nil, err
		}
		// NOT is_external: the external accounts get entries but no balance
		// update, otherwise every transaction would lock the same two rows.
		if _, err := tx.Exec(ctx, `UPDATE accounts SET balance_cents = balance_cents + $2 WHERE id = $1 AND NOT is_external`,
			e.AccountID, e.AmountCents); err != nil {
			return uuid.Nil, err
		}
	}
	return txnID, nil
}
