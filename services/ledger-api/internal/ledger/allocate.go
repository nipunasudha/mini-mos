// Package ledger holds the pure money rules: no database, no HTTP.
package ledger

import (
	"slices"
	"strings"
)

// LivingPot receives whatever the plan rules do not claim.
const LivingPot = "living"

type Rule struct {
	Pot         string
	AmountCents int64
	Priority    int
}

type Share struct {
	Pot         string `json:"pot"`
	AmountCents int64  `json:"amount_cents"`
}

// Allocate fills pots in ascending priority, each up to its rule amount, until
// the deposit runs out. The remainder goes to living. Pots that get nothing
// still appear (with 0) so the response always shows the whole plan.
func Allocate(amount int64, rules []Rule) []Share {
	ordered := slices.Clone(rules) // never reorder the caller's slice
	slices.SortStableFunc(ordered, func(a, b Rule) int { return a.Priority - b.Priority })

	left := amount
	shares := make([]Share, 0, len(ordered)+1)
	for _, r := range ordered {
		n := min(r.AmountCents, left)
		shares = append(shares, Share{r.Pot, n})
		left -= n
	}
	return append(shares, Share{LivingPot, left})
}

// AccountID joins client and pot with ":", which is why ValidClientID bans it.
func AccountID(clientID, pot string) string { return clientID + ":" + pot }

// ValidClientID keeps account IDs unambiguous and URL/log friendly.
func ValidClientID(id string) bool {
	return id != "" && len(id) <= 64 && !strings.ContainsAny(id, ": /\t\n")
}
