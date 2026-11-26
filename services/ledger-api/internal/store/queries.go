package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Pot struct {
	Pot          string `json:"pot"`
	BalanceCents int64  `json:"balance_cents"`
}

type Bill struct {
	BillID      uuid.UUID `json:"bill_id"`
	Pot         string    `json:"pot"`
	Payee       string    `json:"payee"`
	AmountCents int64     `json:"amount_cents"`
	DueDate     string    `json:"due_date"`
	Status      string    `json:"status"`
}

// Pots returns an empty list when the client does not exist (every client has pots).
func (s *Store) Pots(ctx context.Context, clientID string) ([]Pot, error) {
	rows, err := s.pool.Query(ctx, `SELECT pot, balance_cents FROM accounts WHERE client_id = $1 ORDER BY pot`, clientID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Pot])
}

// Bills lists a client's bills by due date. status "" means any; dueBefore nil
// means no date limit, otherwise bills due on or before that day.
func (s *Store) Bills(ctx context.Context, clientID, status string, dueBefore *time.Time) ([]Bill, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, pot, payee, amount_cents, to_char(due_date, 'YYYY-MM-DD'), status FROM bills
		 WHERE client_id = $1 AND ($2 = '' OR status = $2) AND ($3::date IS NULL OR due_date <= $3)
		 ORDER BY due_date, id`, clientID, status, dueBefore)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Bill])
}

func (s *Store) ClientExists(ctx context.Context, clientID string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM clients WHERE id = $1)`, clientID).Scan(&ok)
	return ok, err
}
