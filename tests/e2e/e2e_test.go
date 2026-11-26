//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"reflect"
	"testing"
	"time"
)

// TestE2E fixes the order of the tests.
//
// A top-level test that calls t.Parallel() is paused until every *sequential*
// top-level test has finished, so a plain serial redelivery test would run FIRST.
// Instead: the "parallel" group holds the parallel tests, and t.Run only returns
// once they have all finished. The redelivery test comes after it, so it runs last
// and can snapshot the clients the other tests created.
func TestE2E(t *testing.T) {
	t.Run("parallel", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			fn   func(*testing.T)
		}{
			{"TestDepositIsIdempotent", testDepositIsIdempotent},
			{"TestIdempotencyKeyReuseWithDifferentBodyIsRejected", testIdempotencyKeyReuseWithDifferentBodyIsRejected},
			{"TestShortfallThenPaidAfterSecondDeposit", testShortfallThenPaidAfterSecondDeposit},
			{"TestLedgerBalances", testLedgerBalances},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				tc.fn(t)
			})
		}
	})
	t.Run("TestRedeliveryPaysOnce", testRedeliveryPaysOnce)
}

// Same key twice: the second call replays the first answer and moves no money.
func testDepositIsIdempotent(t *testing.T) {
	c := newClient(t)
	key := "dep-" + randHex(t)

	s1, _, b1 := c.deposit(t, key, 250000)
	if s1 != http.StatusCreated {
		t.Fatalf("first deposit: status %d: %s", s1, b1)
	}
	s2, h2, b2 := c.deposit(t, key, 250000)
	if s2 != http.StatusOK {
		t.Fatalf("replayed deposit: status %d, want 200: %s", s2, b2)
	}
	if h2.Get("Idempotent-Replayed") != "true" {
		t.Errorf("Idempotent-Replayed = %q, want true", h2.Get("Idempotent-Replayed"))
	}
	// Decode before comparing: the replay comes back from a JSONB column, which
	// does not keep key order or whitespace, so the raw bytes may differ.
	var j1, j2 any
	if err := json.Unmarshal(b1, &j1); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b2, &j2); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(j1, j2) {
		t.Errorf("replay body differs:\n first:  %s\n second: %s", b1, b2)
	}

	// The scheduler pays phone (4500) in the background; wait for it so the
	// balances are final. A double-applied deposit would show as doubled pots.
	c.waitSettled(t)
	want := map[string]int64{"rent": 60000, "power": 15000, "phone": 500, "living": 170000}
	if got := c.pots(t); !reflect.DeepEqual(got, want) {
		t.Errorf("pots = %v, want %v", got, want)
	}
}

// Same key, different body: refused, not silently replayed.
func testIdempotencyKeyReuseWithDifferentBodyIsRejected(t *testing.T) {
	c := newClient(t)
	key := "dep-" + randHex(t)

	if s, _, b := c.deposit(t, key, 250000); s != http.StatusCreated {
		t.Fatalf("first deposit: status %d: %s", s, b)
	}
	s, _, b := c.deposit(t, key, 100000)
	if s != http.StatusUnprocessableEntity {
		t.Fatalf("reused key: status %d, want 422: %s", s, b)
	}
	var e struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(b, &e); err != nil || e.Error != "idempotency_key_reused" {
		t.Errorf("error = %q (%v), want idempotency_key_reused: %s", e.Error, err, b)
	}
}

// 250000 pays phone but leaves rent and power short; a second deposit pays them.
func testShortfallThenPaidAfterSecondDeposit(t *testing.T) {
	c := newClient(t)
	rent, power, phone := c.firstBill["rent"], c.firstBill["power"], c.firstBill["phone"]

	c.mustDeposit(t, 250000)
	c.waitPaid(t, phone)
	// The scheduler goes oldest first (rent, power, phone), so phone being paid means
	// it has already tried and failed rent and power. These checks cannot be early.
	paid := c.paidIDs(t)
	if paid[rent] || paid[power] {
		t.Fatalf("rent/power paid after first deposit; rent=%v power=%v", paid[rent], paid[power])
	}

	c.mustDeposit(t, 250000)
	c.waitPaid(t, rent, power, phone)
}

// Money is conserved: pots hold exactly what came in minus what was paid out.
func testLedgerBalances(t *testing.T) {
	const deposited = 250000
	c := newClient(t)
	c.mustDeposit(t, deposited)
	c.waitSettled(t) // no more payments can happen for this deposit

	var inPots, paid int64
	for _, v := range c.pots(t) {
		inPots += v
	}
	for _, b := range c.bills(t, "paid") {
		paid += b.AmountCents
	}
	if inPots != deposited-paid {
		t.Errorf("pots hold %d, want deposited %d - paid %d = %d", inPots, deposited, paid, deposited-paid)
	}
}

type snapshot struct {
	Pots      map[string]int64
	Paid      []bill
	Scheduled []bill
}

func takeSnapshot(t *testing.T, c *client) snapshot {
	t.Helper()
	return snapshot{c.pots(t), c.bills(t, "paid"), c.bills(t, "scheduled")}
}

// Replaying every event must change nothing: no extra payments, no balance moves.
// It runs after the parallel group, so it also covers the other tests' clients.
func testRedeliveryPaysOnce(t *testing.T) {
	if redeliverCmd == "" {
		t.Skip("REDELIVER_CMD not set")
	}
	c := newClient(t)
	c.mustDeposit(t, 250000)

	createdMu.Lock()
	clients := append([]*client(nil), created...)
	createdMu.Unlock()

	// Let the scheduler finish every client's deposit first; otherwise a payment
	// still in flight would look like a change caused by the redelivery.
	before := map[string]snapshot{}
	for _, cl := range clients {
		if cl.deposited {
			cl.waitSettled(t)
		}
		before[cl.id] = takeSnapshot(t, cl)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "sh", "-c", redeliverCmd).CombinedOutput()
	t.Logf("redeliver output:\n%s", out)
	if err != nil {
		t.Fatalf("REDELIVER_CMD failed: %v", err)
	}

	for _, cl := range clients {
		if after := takeSnapshot(t, cl); !reflect.DeepEqual(before[cl.id], after) {
			t.Errorf("client %s changed after redelivery:\n before: %+v\n after:  %+v", cl.id, before[cl.id], after)
		}
	}
}
