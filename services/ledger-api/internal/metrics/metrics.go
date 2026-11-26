// Package metrics defines every Prometheus metric the service exposes.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	// Own registry (not the global one) so tests can build as many as they like.
	Registry *prometheus.Registry

	HTTPRequests    *prometheus.CounterVec
	HTTPDuration    *prometheus.HistogramVec
	Deposits        prometheus.Counter
	DepositCents    prometheus.Counter
	Payments        *prometheus.CounterVec // result: paid|replayed|insufficient_funds|already_paid
	OutboxPending   prometheus.Gauge
	OutboxPublished prometheus.Counter
}

func New() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	f := promauto.With(reg)
	return &Metrics{
		Registry: reg,
		HTTPRequests: f.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total", Help: "HTTP requests by route pattern, method and status code.",
		}, []string{"route", "method", "code"}),
		HTTPDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Name: "http_request_duration_seconds", Help: "HTTP request latency by route pattern and method.",
			Buckets: prometheus.DefBuckets,
		}, []string{"route", "method"}),
		Deposits:     f.NewCounter(prometheus.CounterOpts{Name: "ledger_deposits_total", Help: "Deposits applied (replays excluded)."}),
		DepositCents: f.NewCounter(prometheus.CounterOpts{Name: "ledger_deposit_cents_total", Help: "Cents deposited (replays excluded)."}),
		Payments: f.NewCounterVec(prometheus.CounterOpts{
			Name: "ledger_payments_total", Help: "Payment requests by result.",
		}, []string{"result"}),
		OutboxPending:   f.NewGauge(prometheus.GaugeOpts{Name: "ledger_outbox_pending", Help: "Outbox rows not yet published."}),
		OutboxPublished: f.NewCounter(prometheus.CounterOpts{Name: "ledger_outbox_published_total", Help: "Outbox rows published to Kafka."}),
	}
}
