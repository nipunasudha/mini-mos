// Package api is the HTTP layer: routing, health, metrics and JSON plumbing.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/nipunasudha/mini-mos/services/ledger-api/internal/metrics"
	"github.com/nipunasudha/mini-mos/services/ledger-api/internal/store"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server struct {
	store        *store.Store
	metrics      *metrics.Metrics
	adminEnabled bool
	shuttingDown atomic.Bool
}

func New(st *store.Store, m *metrics.Metrics, adminEnabled bool) *Server {
	return &Server{store: st, metrics: m, adminEnabled: adminEnabled}
}

// StartShutdown makes /readyz fail so the load balancer stops sending traffic.
func (s *Server) StartShutdown() { s.shuttingDown.Store(true) }

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(s.instrument)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }) // never touches the DB
	r.Get("/readyz", s.readyz)
	r.Method(http.MethodGet, "/metrics", promhttp.HandlerFor(s.metrics.Registry, promhttp.HandlerOpts{}))

	r.Post("/v1/clients/{clientID}/deposits", s.deposit)
	r.Get("/v1/clients/{clientID}/pots", s.pots)
	r.Get("/v1/clients/{clientID}/bills", s.bills)
	r.Post("/v1/bills/{billID}/payments", s.pay)
	if s.adminEnabled { // when off the route does not exist, so it 404s like any unknown path
		r.Post("/v1/admin/clients", s.adminCreateClient)
	}
	return r
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	if s.shuttingDown.Load() {
		writeError(w, http.StatusServiceUnavailable, "shutting_down", "server is shutting down")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "db_unavailable", "database ping failed")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// instrument records RED metrics. The label is chi's route pattern
// ("/v1/bills/{billID}/payments"), not the raw path: raw paths contain IDs and
// would create an unbounded number of time series.
func (s *Server) instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		// The pattern is only known once routing is done, hence after next().
		route := chi.RouteContext(r.Context()).RoutePattern()
		if route == "" {
			route = "unmatched"
		}
		code := ww.Status()
		if code == 0 { // handler wrote nothing: net/http sends 200
			code = http.StatusOK
		}
		s.metrics.HTTPRequests.WithLabelValues(route, r.Method, strconv.Itoa(code)).Inc()
		s.metrics.HTTPDuration.WithLabelValues(route, r.Method).Observe(time.Since(start).Seconds())
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write response", "err", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": code, "message": msg})
}

func internalError(w http.ResponseWriter, err error) {
	slog.Error("request failed", "err", err)
	writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
}

// writeResult sends a stored or fresh store.Result as-is (the body is already JSON).
func writeResult(w http.ResponseWriter, res store.Result) {
	w.Header().Set("Content-Type", "application/json")
	if res.Replayed {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	w.WriteHeader(res.Status)
	w.Write(res.Body)
}
