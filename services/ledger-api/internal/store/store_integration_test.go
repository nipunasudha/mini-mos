//go:build integration

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	testStore *Store
	testPool  *pgxpool.Pool
)

// One throwaway Postgres for the whole package; each test seeds its own client,
// so tests do not see each other's rows.
func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
	ctx := context.Background()
	pg, err := tcpostgres.Run(ctx, "postgres:19.0-alpine",
		tcpostgres.WithDatabase("ledger"), tcpostgres.WithUsername("app"), tcpostgres.WithPassword("app"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)))
	if err != nil {
		fmt.Fprintln(os.Stderr, "start postgres:", err)
		return 1
	}
	defer func() { _ = testcontainers.TerminateContainer(pg) }()

	url, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err == nil {
		err = Migrate(ctx, url)
	}
	if err == nil {
		testPool, err = pgxpool.New(ctx, url)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "setup:", err)
		return 1
	}
	defer testPool.Close()
	testStore = New(testPool)
	return m.Run()
}

func newClient(t *testing.T) string {
	t.Helper()
	id := "t-" + uuid.NewString()[:8]
	if _, err := testStore.SeedClient(context.Background(), id, "Test"); err != nil {
		t.Fatal(err)
	}
	return id
}

func count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func potBalances(t *testing.T, clientID string) map[string]int64 {
	t.Helper()
	pots, err := testStore.Pots(context.Background(), clientID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, p := range pots {
		got[p.Pot] = p.BalanceCents
	}
	return got
}

// firstBill returns the earliest scheduled bill of a pot.
func firstBill(t *testing.T, clientID, pot string) Bill {
	t.Helper()
	bills, err := testStore.Bills(context.Background(), clientID, "scheduled", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bills {
		if b.Pot == pot {
			return b
		}
	}
	t.Fatalf("no scheduled %s bill", pot)
	return Bill{}
}

func deposit(t *testing.T, clientID, key string, cents int64) Result {
	t.Helper()
	res, err := testStore.Deposit(context.Background(), clientID, key, DepositRequest{AmountCents: cents, Source: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestConcurrentDuplicateDepositKeysHaveOneEffect(t *testing.T) {
	clientID := newClient(t)
	const callers = 8

	results := make([]Result, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], _ = testStore.Deposit(context.Background(), clientID, "dup-key", DepositRequest{AmountCents: 250000, Source: "test"})
		}()
	}
	wg.Wait()

	var created, replayed int
	for _, r := range results {
		switch {
		case r.Status == 201 && !r.Replayed:
			created++
		case r.Status == 200 && r.Replayed:
			replayed++
			if string(r.Body) != string(results[indexOfCreated(results)].Body) {
				t.Errorf("replayed body differs from original:\n%s\n%s", r.Body, results[indexOfCreated(results)].Body)
			}
		default:
			t.Errorf("unexpected result: status=%d replayed=%v body=%s", r.Status, r.Replayed, r.Body)
		}
	}
	if created != 1 || replayed != callers-1 {
		t.Fatalf("created=%d replayed=%d, want 1 and %d", created, replayed, callers-1)
	}

	if n := count(t, `SELECT count(*) FROM deposits WHERE client_id = $1`, clientID); n != 1 {
		t.Errorf("deposit rows = %d, want 1", n)
	}
	want := map[string]int64{"rent": 60000, "power": 15000, "phone": 5000, "living": 170000}
	for pot, cents := range want {
		if got := potBalances(t, clientID)[pot]; got != cents {
			t.Errorf("pot %s = %d, want %d (moved more than once?)", pot, got, cents)
		}
	}
	if n := count(t, `SELECT count(*) FROM outbox WHERE msg_key = $1`, clientID); n != 1 {
		t.Errorf("outbox rows = %d, want 1", n)
	}
	assertEntriesBalance(t, clientID)
}

func indexOfCreated(rs []Result) int {
	for i, r := range rs {
		if !r.Replayed {
			return i
		}
	}
	return 0
}

func TestSameKeyDifferentBodyIsRejected(t *testing.T) {
	clientID := newClient(t)
	deposit(t, clientID, "k", 1000)
	res := deposit(t, clientID, "k", 2000)
	if res.Status != 422 || res.Err != "idempotency_key_reused" {
		t.Fatalf("got status=%d err=%q", res.Status, res.Err)
	}
}

func TestSecondPaymentOfPaidBillWithNewKeyIsRejected(t *testing.T) {
	clientID := newClient(t)
	deposit(t, clientID, "d1", 250000)
	phone := firstBill(t, clientID, "phone")
	ctx := context.Background()

	first, err := testStore.Pay(ctx, phone.BillID, "pay-1")
	if err != nil || first.Status != 201 {
		t.Fatalf("first payment: %v status=%d body=%s", err, first.Status, first.Body)
	}
	second, err := testStore.Pay(ctx, phone.BillID, "pay-2") // NEW key
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != 409 || second.Err != "bill_already_paid" {
		t.Fatalf("second payment: status=%d err=%q", second.Status, second.Err)
	}
	// Retrying the first key replays the original 201 as a 200.
	again, _ := testStore.Pay(ctx, phone.BillID, "pay-1")
	if again.Status != 200 || !again.Replayed {
		t.Fatalf("replay: status=%d replayed=%v", again.Status, again.Replayed)
	}
	if n := count(t, `SELECT count(*) FROM payments WHERE bill_id = $1`, phone.BillID); n != 1 {
		t.Errorf("payment rows = %d, want 1", n)
	}
	if got := potBalances(t, clientID)["phone"]; got != 500 {
		t.Errorf("phone pot = %d, want 500", got)
	}
	assertEntriesBalance(t, clientID)
}

func TestInsufficientFundsStoresNoKey(t *testing.T) {
	clientID := newClient(t)
	rent := firstBill(t, clientID, "rent")
	ctx := context.Background()

	res, err := testStore.Pay(ctx, rent.BillID, "pay-rent")
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != 409 || res.Err != "insufficient_funds" {
		t.Fatalf("status=%d err=%q", res.Status, res.Err)
	}
	var body struct {
		NeededCents    int64 `json:"needed_cents"`
		AvailableCents int64 `json:"available_cents"`
	}
	if err := json.Unmarshal(res.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body.NeededCents != 120000 || body.AvailableCents != 0 {
		t.Errorf("body = %s", res.Body)
	}
	if n := count(t, `SELECT count(*) FROM idempotency_keys WHERE scope = $1`, "payment:"+rent.BillID.String()); n != 0 {
		t.Fatalf("idempotency_keys rows = %d, want 0 (key must roll back)", n)
	}

	// Money arrives (two deposits fill rent to 120000); the SAME key now succeeds.
	deposit(t, clientID, "d1", 250000)
	deposit(t, clientID, "d2", 250000)
	res, err = testStore.Pay(ctx, rent.BillID, "pay-rent")
	if err != nil || res.Status != 201 || res.Replayed {
		t.Fatalf("retry after funds: err=%v status=%d replayed=%v", err, res.Status, res.Replayed)
	}
	assertEntriesBalance(t, clientID)
}

func TestSeedIsIdempotent(t *testing.T) {
	ctx := context.Background()
	id := "t-" + uuid.NewString()[:8]
	if created, err := testStore.SeedClient(ctx, id, "X"); err != nil || !created {
		t.Fatalf("first seed: created=%v err=%v", created, err)
	}
	if created, err := testStore.SeedClient(ctx, id, "X"); err != nil || created {
		t.Fatalf("second seed: created=%v err=%v", created, err)
	}
	if n := count(t, `SELECT count(*) FROM bills WHERE client_id = $1`, id); n != 4 {
		t.Fatalf("bills = %d, want 4", n)
	}
}

// Every ledger transaction of the client must sum to zero.
func assertEntriesBalance(t *testing.T, clientID string) {
	t.Helper()
	n := count(t, `SELECT count(*) FROM (
		SELECT e.txn_id FROM ledger_entries e JOIN ledger_txns x ON x.id = e.txn_id
		WHERE x.client_id = $1 GROUP BY e.txn_id HAVING sum(e.amount_cents) <> 0) bad`, clientID)
	if n != 0 {
		t.Errorf("%d ledger txns do not sum to zero", n)
	}
}
