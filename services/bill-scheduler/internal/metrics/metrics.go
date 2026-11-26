// Package metrics holds the Prometheus collectors for bill-scheduler.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

type Metrics struct {
	Registry          *prometheus.Registry
	Events            *prometheus.CounterVec // result: ok|dlq
	PaymentAttempts   *prometheus.CounterVec // result: paid|replayed|already_paid|insufficient_funds|error
	ShortfallsPublish prometheus.Counter
}

// New builds a private registry (not the global one) so tests can create
// as many instances as they like without duplicate-registration panics.
func New() *Metrics {
	m := &Metrics{
		Registry: prometheus.NewRegistry(),
		Events: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "scheduler_events_total",
			Help: "deposits.allocated records handled, by outcome.",
		}, []string{"result"}),
		PaymentAttempts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "scheduler_payment_attempts_total",
			Help: "Bill payment calls to ledger-api, by final result.",
		}, []string{"result"}),
		ShortfallsPublish: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "scheduler_shortfalls_published_total",
			Help: "pots.shortfall events published.",
		}),
	}
	m.Registry.MustRegister(m.Events, m.PaymentAttempts, m.ShortfallsPublish,
		collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return m
}
