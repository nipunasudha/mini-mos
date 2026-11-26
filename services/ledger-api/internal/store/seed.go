package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/nipunasudha/mini-mos/services/ledger-api/internal/ledger"
)

// billNamespace makes bill IDs deterministic: the same client, pot and due
// offset always give the same UUID, so ON CONFLICT DO NOTHING really does
// make re-seeding a no-op instead of adding a second set of bills.
var billNamespace = uuid.MustParse("6f1c0b0e-3a52-4d0e-9b57-2a1d6c8e4f10")

var seedPots = []string{"rent", "power", "phone", ledger.LivingPot}

var seedRules = []ledger.Rule{{Pot: "rent", AmountCents: 60000, Priority: 1}, {Pot: "power", AmountCents: 15000, Priority: 2}, {Pot: "phone", AmountCents: 5000, Priority: 3}}

var seedBills = []struct {
	pot, payee  string
	amountCents int64
	dueInDays   int
}{
	{"rent", "Landlord", 120000, 2},
	{"power", "Energy Co", 18000, 4},
	{"phone", "Telco", 4500, 6},
	{"rent", "Landlord", 120000, 16},
}

// SeedClient creates a client with the standard pots, plan and bills. It is the
// only seed code: the `seed` subcommand and POST /v1/admin/clients both call it.
// Returns true if the client was new.
func (s *Store) SeedClient(ctx context.Context, clientID, name string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	// A concurrent seed of the same ID blocks here until we commit, then sees 0 rows.
	tag, err := tx.Exec(ctx, `INSERT INTO clients (id, name) VALUES ($1, $2) ON CONFLICT DO NOTHING`, clientID, name)
	if err != nil {
		return false, err
	}
	created := tag.RowsAffected() == 1

	for _, pot := range seedPots {
		if _, err := tx.Exec(ctx, `INSERT INTO accounts (id, client_id, pot) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
			ledger.AccountID(clientID, pot), clientID, pot); err != nil {
			return false, err
		}
	}
	for _, r := range seedRules {
		if _, err := tx.Exec(ctx, `INSERT INTO plan_rules (client_id, pot, amount_cents, priority) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
			clientID, r.Pot, r.AmountCents, r.Priority); err != nil {
			return false, err
		}
	}
	for _, b := range seedBills {
		id := uuid.NewSHA1(billNamespace, []byte(fmt.Sprintf("%s|%s|%d", clientID, b.pot, b.dueInDays)))
		if _, err := tx.Exec(ctx,
			`INSERT INTO bills (id, client_id, pot, payee, amount_cents, due_date)
			 VALUES ($1, $2, $3, $4, $5, CURRENT_DATE + $6::int) ON CONFLICT DO NOTHING`,
			id, clientID, b.pot, b.payee, b.amountCents, b.dueInDays); err != nil {
			return false, err
		}
	}
	return created, tx.Commit(ctx)
}
