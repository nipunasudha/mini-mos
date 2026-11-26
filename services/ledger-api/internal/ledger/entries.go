package ledger

const (
	IncomeAccount = "external:income"
	PayeesAccount = "external:payees"
)

// Entry is one leg of a ledger transaction: +credit / -debit.
type Entry struct {
	AccountID   string
	AmountCents int64
}

// DepositEntries debits external:income and credits each pot that got money.
// Zero shares are skipped because ledger_entries forbids zero amounts.
func DepositEntries(clientID string, amount int64, shares []Share) []Entry {
	entries := []Entry{{IncomeAccount, -amount}}
	for _, s := range shares {
		if s.AmountCents > 0 {
			entries = append(entries, Entry{AccountID(clientID, s.Pot), s.AmountCents})
		}
	}
	return entries
}

// PaymentEntries debits the pot and credits external:payees.
func PaymentEntries(clientID, pot string, amount int64) []Entry {
	return []Entry{{AccountID(clientID, pot), -amount}, {PayeesAccount, amount}}
}

// Sum must be 0 for every transaction (double-entry invariant).
func Sum(entries []Entry) int64 {
	var total int64
	for _, e := range entries {
		total += e.AmountCents
	}
	return total
}
