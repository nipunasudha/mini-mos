//go:build e2e

package e2e

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"testing"
	"time"
)

type pot struct {
	Pot          string `json:"pot"`
	BalanceCents int64  `json:"balance_cents"`
}

type bill struct {
	BillID      string `json:"bill_id"`
	Pot         string `json:"pot"`
	Payee       string `json:"payee"`
	AmountCents int64  `json:"amount_cents"`
	DueDate     string `json:"due_date"`
	Status      string `json:"status"`
}

// client is one throwaway client; every test makes its own so tests never share state.
type client struct {
	id        string
	deposited bool              // set once a deposit has been accepted; the redelivery test needs it
	firstBill map[string]string // pot -> ID of its soonest-due bill (a client has two rent bills)
}

// created remembers every client so the redelivery test can check them all.
var (
	createdMu sync.Mutex
	created   []*client
)

func randHex(t *testing.T) string {
	t.Helper()
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

// call sends one request and returns status, headers and the raw body.
func call(t *testing.T, method, path, idemKey string, body any) (int, http.Header, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, baseURL+path, r)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	resp, err := httpc.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, resp.Header, b
}

// newClient creates a fresh client (seeded with pots, plan and 4 bills) via the admin endpoint.
func newClient(t *testing.T) *client {
	t.Helper()
	c := &client{id: "e2e-" + randHex(t)}
	status, _, b := call(t, "POST", "/v1/admin/clients", "", map[string]string{"client_id": c.id, "name": "E2E client"})
	if status != http.StatusCreated {
		t.Fatalf("create client %s: status %d: %s", c.id, status, b)
	}
	// Read the bill IDs now, while nothing can have been paid yet.
	c.firstBill = map[string]string{}
	for _, b := range c.bills(t, "scheduled") { // sorted by due date
		if _, seen := c.firstBill[b.Pot]; !seen {
			c.firstBill[b.Pot] = b.BillID
		}
	}
	createdMu.Lock()
	created = append(created, c)
	createdMu.Unlock()
	return c
}

// deposit posts a deposit and returns the raw response. It does not judge the status.
func (c *client) deposit(t *testing.T, key string, cents int64) (int, http.Header, []byte) {
	t.Helper()
	status, h, b := call(t, "POST", "/v1/clients/"+c.id+"/deposits", key,
		map[string]any{"amount_cents": cents, "source": "e2e"})
	if status == http.StatusCreated {
		c.deposited = true
	}
	return status, h, b
}

// mustDeposit makes a deposit with a fresh key and fails the test unless it is accepted.
func (c *client) mustDeposit(t *testing.T, cents int64) {
	t.Helper()
	if status, _, b := c.deposit(t, "dep-"+randHex(t), cents); status != http.StatusCreated {
		t.Fatalf("deposit: status %d: %s", status, b)
	}
}

func getJSON(t *testing.T, path string, out any) {
	t.Helper()
	status, _, b := call(t, "GET", path, "", nil)
	if status != http.StatusOK {
		t.Fatalf("GET %s: status %d: %s", path, status, b)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("GET %s: bad JSON %q: %v", path, b, err)
	}
}

func (c *client) pots(t *testing.T) map[string]int64 {
	t.Helper()
	var ps []pot
	getJSON(t, "/v1/clients/"+c.id+"/pots", &ps)
	m := map[string]int64{}
	for _, p := range ps {
		m[p.Pot] = p.BalanceCents
	}
	return m
}

// bills returns bills with the given status, sorted so two reads compare equal.
func (c *client) bills(t *testing.T, status string) []bill {
	t.Helper()
	var bs []bill
	getJSON(t, "/v1/clients/"+c.id+"/bills?status="+status, &bs)
	sort.Slice(bs, func(i, j int) bool {
		if bs[i].DueDate != bs[j].DueDate {
			return bs[i].DueDate < bs[j].DueDate
		}
		return bs[i].BillID < bs[j].BillID
	})
	return bs
}

// paidIDs is the set of bill IDs that are paid right now.
func (c *client) paidIDs(t *testing.T) map[string]bool {
	t.Helper()
	m := map[string]bool{}
	for _, b := range c.bills(t, "paid") {
		m[b.BillID] = true
	}
	return m
}

// waitPaid waits until every given bill is paid.
func (c *client) waitPaid(t *testing.T, ids ...string) {
	t.Helper()
	eventually(t, timeout, 500*time.Millisecond, func() error {
		paid := c.paidIDs(t)
		for _, id := range ids {
			if !paid[id] {
				return fmt.Errorf("bill %s not paid yet", id)
			}
		}
		return nil
	})
}

// waitSettled waits for the scheduler to finish the first deposit event. It pays
// bills oldest first (rent, power, phone), so once phone is paid it has nothing left
// to do for that event and balances stop moving.
func (c *client) waitSettled(t *testing.T) {
	t.Helper()
	c.waitPaid(t, c.firstBill["phone"])
}
