package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/nipunasudha/mini-mos/services/bill-scheduler/internal/ledgerclient"
	"github.com/nipunasudha/mini-mos/services/bill-scheduler/internal/metrics"
)

const depositEvent = `{"event_id":"e-1","type":"deposits.allocated","client_id":"c-1","data":{"deposit_id":"d-1"}}`

const billsJSON = `[{"bill_id":"b-1","pot":"rent","payee":"Landlord","amount_cents":120000,"due_date":"2026-12-01","status":"scheduled"}]`

// fakeLedger serves one bill and answers payments with pay(callNumber).
func fakeLedger(t *testing.T, pay func(n int32, w http.ResponseWriter)) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var payCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/clients/{id}/bills", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("status") != "scheduled" || r.URL.Query().Get("due_before") != "2026-12-03" {
			t.Errorf("unexpected query %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(billsJSON))
	})
	mux.HandleFunc("POST /v1/bills/{id}/payments", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Idempotency-Key"); got != "bill-b-1-2026-12-01" {
			t.Errorf("Idempotency-Key = %q", got)
		}
		pay(payCalls.Add(1), w)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &payCalls
}

func reply(w http.ResponseWriter, status int, body string) {
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func TestHandle(t *testing.T) {
	const shortBody = `{"error":"insufficient_funds","pot":"rent","needed_cents":120000,"available_cents":60000}`
	tests := []struct {
		name          string
		pay           func(n int32, w http.ResponseWriter)
		wantErr       bool
		wantPayCalls  int32
		wantShortfall bool
		wantResult    string // payment_attempts label that should be 1
	}{
		{"201 paid", func(_ int32, w http.ResponseWriter) { reply(w, 201, `{}`) }, false, 1, false, "paid"},
		{"200 replayed", func(_ int32, w http.ResponseWriter) { reply(w, 200, `{}`) }, false, 1, false, "replayed"},
		{"409 already paid", func(_ int32, w http.ResponseWriter) { reply(w, 409, `{"error":"bill_already_paid"}`) }, false, 1, false, "already_paid"},
		{"409 insufficient funds", func(_ int32, w http.ResponseWriter) { reply(w, 409, shortBody) }, false, 1, true, "insufficient_funds"},
		{"5xx then success", func(n int32, w http.ResponseWriter) {
			if n < 3 {
				reply(w, 503, `{}`)
				return
			}
			reply(w, 201, `{}`)
		}, false, 3, false, "paid"},
		{"5xx always", func(_ int32, w http.ResponseWriter) { reply(w, 500, `{}`) }, true, maxAttempts, false, "error"},
		{"other 4xx is final", func(_ int32, w http.ResponseWriter) { reply(w, 422, `{"error":"idempotency_key_reused"}`) }, true, 1, false, "error"},
		{"unknown 409 is final", func(_ int32, w http.ResponseWriter) { reply(w, 409, `{"error":"something_else"}`) }, true, 1, false, "error"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, payCalls := fakeLedger(t, tc.pay)
			m := metrics.New()
			var got []Event
			h := &Handler{
				Ledger:  ledgerclient.New(srv.URL),
				Metrics: m,
				Publish: func(_ context.Context, clientID string, ev Event) error {
					if clientID != "c-1" {
						t.Errorf("shortfall key = %q", clientID)
					}
					got = append(got, ev)
					return nil
				},
				HorizonDays: 7,
				Now:         func() time.Time { return time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC) },
				RetryBase:   time.Millisecond,
			}

			err := h.Handle(context.Background(), []byte(depositEvent))

			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if payCalls.Load() != tc.wantPayCalls {
				t.Errorf("payment calls = %d, want %d", payCalls.Load(), tc.wantPayCalls)
			}
			if c := testutil.ToFloat64(m.PaymentAttempts.WithLabelValues(tc.wantResult)); c != 1 {
				t.Errorf("payment_attempts{%s} = %v, want 1", tc.wantResult, c)
			}
			if (len(got) == 1) != tc.wantShortfall || len(got) > 1 {
				t.Fatalf("published %d shortfalls, want shortfall=%v", len(got), tc.wantShortfall)
			}
			if tc.wantShortfall {
				var s Shortfall
				if err := json.Unmarshal(got[0].Data, &s); err != nil {
					t.Fatal(err)
				}
				want := Shortfall{"b-1", "rent", "Landlord", "2026-12-01", 120000, 60000, "d-1"}
				if s != want {
					t.Errorf("shortfall = %+v, want %+v", s, want)
				}
				if got[0].EventID != shortfallID("b-1", "2026-12-01", "d-1") || got[0].Type != "pots.shortfall" || got[0].ClientID != "c-1" {
					t.Errorf("bad envelope %+v", got[0])
				}
				if c := testutil.ToFloat64(m.ShortfallsPublish); c != 1 {
					t.Errorf("shortfalls_published = %v, want 1", c)
				}
			}
		})
	}
}

func TestHandleMalformedNeverCallsLedger(t *testing.T) {
	srv, payCalls := fakeLedger(t, func(int32, http.ResponseWriter) {})
	h := &Handler{Ledger: ledgerclient.New(srv.URL), Metrics: metrics.New(), Now: time.Now}
	err := h.Handle(context.Background(), []byte(`{not json`))
	if !errors.Is(err, errMalformed) || payCalls.Load() != 0 {
		t.Fatalf("err = %v, pay calls = %d", err, payCalls.Load())
	}
}

func TestHandlePublishFailureFailsRecord(t *testing.T) {
	srv, _ := fakeLedger(t, func(_ int32, w http.ResponseWriter) {
		reply(w, 409, `{"error":"insufficient_funds","needed_cents":2,"available_cents":1}`)
	})
	h := &Handler{
		Ledger: ledgerclient.New(srv.URL), Metrics: metrics.New(), HorizonDays: 7, RetryBase: time.Millisecond,
		Publish: func(context.Context, string, Event) error { return errors.New("broker down") },
		Now:     func() time.Time { return time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC) },
	}
	if err := h.Handle(context.Background(), []byte(depositEvent)); err == nil {
		t.Fatal("want error when the shortfall cannot be published")
	}
}
