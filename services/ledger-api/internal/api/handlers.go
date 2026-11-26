package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nipunasudha/mini-mos/services/ledger-api/internal/ledger"
	"github.com/nipunasudha/mini-mos/services/ledger-api/internal/store"
)

const maxBody = 1 << 20

func (s *Server) deposit(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "missing_idempotency_key", "Idempotency-Key header is required")
		return
	}
	var req store.DepositRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "body must be JSON")
		return
	}
	if req.AmountCents <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_amount", "amount_cents must be positive")
		return
	}

	res, err := s.store.Deposit(r.Context(), chi.URLParam(r, "clientID"), key, req)
	if err != nil {
		internalError(w, err)
		return
	}
	if res.Status == http.StatusCreated && !res.Replayed {
		s.metrics.Deposits.Inc()
		s.metrics.DepositCents.Add(float64(req.AmountCents))
	}
	writeResult(w, res)
}

func (s *Server) pay(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "missing_idempotency_key", "Idempotency-Key header is required")
		return
	}
	billID, err := uuid.Parse(chi.URLParam(r, "billID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_bill_id", "bill ID must be a UUID")
		return
	}

	res, err := s.store.Pay(r.Context(), billID, key)
	if err != nil {
		internalError(w, err)
		return
	}
	switch {
	case res.Replayed:
		s.metrics.Payments.WithLabelValues("replayed").Inc()
	case res.Status == http.StatusCreated:
		s.metrics.Payments.WithLabelValues("paid").Inc()
	case res.Err == "insufficient_funds":
		s.metrics.Payments.WithLabelValues("insufficient_funds").Inc()
	case res.Err == "bill_already_paid":
		s.metrics.Payments.WithLabelValues("already_paid").Inc()
	}
	writeResult(w, res)
}

func (s *Server) pots(w http.ResponseWriter, r *http.Request) {
	pots, err := s.store.Pots(r.Context(), chi.URLParam(r, "clientID"))
	if err != nil {
		internalError(w, err)
		return
	}
	if len(pots) == 0 {
		writeError(w, http.StatusNotFound, "client_not_found", "no such client")
		return
	}
	writeJSON(w, http.StatusOK, pots)
}

func (s *Server) bills(w http.ResponseWriter, r *http.Request) {
	clientID := chi.URLParam(r, "clientID")
	var dueBefore *time.Time
	if v := r.URL.Query().Get("due_before"); v != "" {
		d, err := time.Parse(time.DateOnly, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_due_before", "due_before must be YYYY-MM-DD")
			return
		}
		dueBefore = &d
	}
	ok, err := s.store.ClientExists(r.Context(), clientID)
	if err != nil {
		internalError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "client_not_found", "no such client")
		return
	}
	bills, err := s.store.Bills(r.Context(), clientID, r.URL.Query().Get("status"), dueBefore)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, bills)
}

func (s *Server) adminCreateClient(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientID string `json:"client_id"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil || !ledger.ValidClientID(req.ClientID) {
		writeError(w, http.StatusBadRequest, "invalid_client", "client_id is required (no ':', '/' or spaces, max 64 chars)")
		return
	}
	created, err := s.store.SeedClient(r.Context(), req.ClientID, req.Name)
	if err != nil {
		internalError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]string{"client_id": req.ClientID, "name": req.Name})
}
