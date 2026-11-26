// Package outbox writes events in the same transaction as the money movement
// and relays them to Kafka afterwards, so an event exists if and only if the
// movement committed.
package outbox

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Envelope is the common event shape from the spec (Section 6).
type Envelope struct {
	EventID    uuid.UUID `json:"event_id"`
	Type       string    `json:"type"`
	OccurredAt time.Time `json:"occurred_at"`
	ClientID   string    `json:"client_id"`
	Data       any       `json:"data"`
}

// Insert adds one event to the outbox inside the caller's transaction. The
// event type doubles as the topic name, and the client ID is the Kafka key so
// one client's events stay ordered within a partition.
func Insert(ctx context.Context, tx pgx.Tx, eventType, clientID string, data any) error {
	env := Envelope{uuid.New(), eventType, time.Now().UTC(), clientID, data}
	payload, err := json.Marshal(env)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO outbox (event_id, topic, msg_key, payload) VALUES ($1, $2, $3, $4)`,
		env.EventID, eventType, clientID, payload)
	return err
}
