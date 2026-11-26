package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
)

// Result is what an HTTP handler needs to answer the caller.
type Result struct {
	Status   int
	Body     []byte
	Replayed bool   // true when served from a stored idempotency key
	Err      string // error code for non-2xx results, "" otherwise (used for metrics)
}

// Rejection is returned by the work function to abort the transaction: nothing
// is committed and the idempotency key is NOT stored, so a retry runs again.
type Rejection struct {
	Status int
	Code   string
	Body   []byte
}

func (r *Rejection) Error() string { return r.Code }

func errBody(code, msg string) []byte {
	b, _ := json.Marshal(map[string]string{"error": code, "message": msg})
	return b
}

func reject(status int, code, msg string) *Rejection {
	return &Rejection{status, code, errBody(code, msg)}
}

// Hash fingerprints a request. Callers pass the PARSED body (a struct), never
// the raw bytes, so whitespace and key order in the original JSON don't matter.
func Hash(v any) string {
	b, _ := json.Marshal(v) // marshalling plain structs and strings cannot fail
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// idempotent runs work at most once per (scope, key).
//
// The key row is inserted first with placeholder values. A concurrent request
// with the same key blocks on that insert until we commit or roll back, so
// duplicates are serialised by the primary key. If we commit, the waiter's
// insert inserts nothing and it replays our stored response. If we roll back
// (a Rejection), the waiter's insert succeeds and it does the work itself.
func (s *Store) idempotent(ctx context.Context, scope, key, hash string, work func(pgx.Tx) (Result, error)) (Result, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(ctx) // no-op after Commit

	tag, err := tx.Exec(ctx,
		`INSERT INTO idempotency_keys (scope, key, request_hash, response_status, response_body)
		 VALUES ($1, $2, $3, 0, '{}') ON CONFLICT DO NOTHING`, scope, key, hash)
	if err != nil {
		return Result{}, err
	}
	if tag.RowsAffected() == 0 {
		// READ COMMITTED (the default): this SELECT is a new statement, so it
		// sees the row the other transaction just committed.
		return replay(ctx, tx, scope, key, hash)
	}

	res, err := work(tx)
	var rej *Rejection
	if errors.As(err, &rej) {
		return Result{Status: rej.Status, Body: rej.Body, Err: rej.Code}, nil
	}
	if err != nil {
		return Result{}, err
	}

	// RETURNING gives back the JSONB-normalised body. We answer with exactly
	// that, so the first response and every replay are byte-identical.
	err = tx.QueryRow(ctx,
		`UPDATE idempotency_keys SET response_status = $3, response_body = $4
		 WHERE scope = $1 AND key = $2 RETURNING response_body`,
		scope, key, res.Status, res.Body).Scan(&res.Body)
	if err != nil {
		return Result{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return res, nil
}

func replay(ctx context.Context, tx pgx.Tx, scope, key, hash string) (Result, error) {
	var storedHash string
	var res Result
	err := tx.QueryRow(ctx,
		`SELECT request_hash, response_status, response_body FROM idempotency_keys WHERE scope = $1 AND key = $2`,
		scope, key).Scan(&storedHash, &res.Status, &res.Body)
	if err != nil {
		return Result{}, fmt.Errorf("read stored key: %w", err)
	}
	if storedHash != hash {
		return Result{Status: http.StatusUnprocessableEntity, Err: "idempotency_key_reused",
			Body: errBody("idempotency_key_reused", "Idempotency-Key was already used with a different request")}, nil
	}
	if res.Status == http.StatusCreated {
		res.Status = http.StatusOK
	}
	res.Replayed = true
	return res, nil
}
