// Package ledgerclient is the small slice of the ledger-api HTTP API that
// bill-scheduler needs.
package ledgerclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ErrTransient marks failures worth retrying: network errors and 5xx.
// Everything else (other 4xx, bad bodies) is final.
var ErrTransient = errors.New("transient ledger error")

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string) *Client {
	// A timeout stops a hung ledger-api from stalling the consumer forever.
	return &Client{baseURL: baseURL, http: &http.Client{Timeout: 10 * time.Second}}
}

type Bill struct {
	BillID      string `json:"bill_id"`
	Pot         string `json:"pot"`
	Payee       string `json:"payee"`
	AmountCents int64  `json:"amount_cents"`
	DueDate     string `json:"due_date"`
	Status      string `json:"status"`
}

// Payment outcomes that the caller treats as non-errors.
const (
	Paid              = "paid"
	Replayed          = "replayed"
	AlreadyPaid       = "already_paid"
	InsufficientFunds = "insufficient_funds"
)

type PayResult struct {
	Result         string
	NeededCents    int64
	AvailableCents int64
}

// Bills lists the client's scheduled bills due on or before dueBefore,
// oldest first (ledger-api sorts them).
func (c *Client) Bills(ctx context.Context, clientID, dueBefore string) ([]Bill, error) {
	q := url.Values{"status": {"scheduled"}, "due_before": {dueBefore}}
	u := fmt.Sprintf("%s/v1/clients/%s/bills?%s", c.baseURL, url.PathEscape(clientID), q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	status, body, err := c.do(req)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, statusErr(status, body)
	}
	var bills []Bill
	if err := json.Unmarshal(body, &bills); err != nil {
		return nil, fmt.Errorf("decode bills: %w", err)
	}
	return bills, nil
}

// Pay makes one attempt. The idempotency key is what makes retries and
// redelivered events safe: ledger-api replays the first answer.
func (c *Client) Pay(ctx context.Context, billID, idempotencyKey string) (PayResult, error) {
	u := fmt.Sprintf("%s/v1/bills/%s/payments", c.baseURL, url.PathEscape(billID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return PayResult{}, err
	}
	req.Header.Set("Idempotency-Key", idempotencyKey)
	status, body, err := c.do(req)
	if err != nil {
		return PayResult{}, err
	}

	switch status {
	case http.StatusCreated:
		return PayResult{Result: Paid}, nil
	case http.StatusOK:
		return PayResult{Result: Replayed}, nil
	case http.StatusConflict:
		var e struct {
			Error          string `json:"error"`
			NeededCents    int64  `json:"needed_cents"`
			AvailableCents int64  `json:"available_cents"`
		}
		if json.Unmarshal(body, &e) == nil {
			switch e.Error {
			case "bill_already_paid":
				return PayResult{Result: AlreadyPaid}, nil
			case "insufficient_funds":
				return PayResult{Result: InsufficientFunds, NeededCents: e.NeededCents, AvailableCents: e.AvailableCents}, nil
			}
		}
	}
	return PayResult{}, statusErr(status, body)
}

// do sends the request and returns status and body. Network errors and 5xx
// come back wrapped in ErrTransient.
func (c *Client) do(req *http.Request) (int, []byte, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		if req.Context().Err() != nil {
			return 0, nil, err // we were cancelled; retrying is pointless
		}
		return 0, nil, fmt.Errorf("%w: %v", ErrTransient, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, nil, fmt.Errorf("%w: read body: %v", ErrTransient, err)
	}
	return resp.StatusCode, body, nil
}

func statusErr(status int, body []byte) error {
	if status >= 500 {
		return fmt.Errorf("%w: ledger returned %d", ErrTransient, status)
	}
	return fmt.Errorf("ledger returned %d: %s", status, body)
}
