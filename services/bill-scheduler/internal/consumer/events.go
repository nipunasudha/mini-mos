package consumer

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Event is the common envelope from spec section 6.
type Event struct {
	EventID    string          `json:"event_id"`
	Type       string          `json:"type"`
	OccurredAt time.Time       `json:"occurred_at"`
	ClientID   string          `json:"client_id"`
	Data       json.RawMessage `json:"data"`
}

type depositData struct {
	DepositID string `json:"deposit_id"`
}

// parseDeposit returns the client and deposit IDs. Anything without them
// can never be handled, so it counts as malformed (and goes to the DLQ).
func parseDeposit(value []byte) (clientID, depositID string, err error) {
	var ev Event
	if err = json.Unmarshal(value, &ev); err != nil {
		return "", "", err
	}
	var d depositData
	if err = json.Unmarshal(ev.Data, &d); err != nil {
		return "", "", err
	}
	if ev.ClientID == "" || d.DepositID == "" {
		return "", "", errors.New("missing client_id or deposit_id")
	}
	return ev.ClientID, d.DepositID, nil
}

type Shortfall struct {
	BillID         string `json:"bill_id"`
	Pot            string `json:"pot"`
	Payee          string `json:"payee"`
	DueDate        string `json:"due_date"`
	NeededCents    int64  `json:"needed_cents"`
	AvailableCents int64  `json:"available_cents"`
	DepositID      string `json:"deposit_id"`
}

// shortfallID is deterministic, so reprocessing the same deposit yields the
// same event_id and the notifier can dedupe on it.
func shortfallID(billID, dueDate, depositID string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(billID+"|"+dueDate+"|"+depositID)).String()
}
