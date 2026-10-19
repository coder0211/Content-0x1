// Package metrics owns the Prometheus registry and the metric families of
// SPEC-02 §8. Modules record through the exported collectors.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds every collector.
type Metrics struct {
	Registry *prometheus.Registry

	HTTPDuration     *prometheus.HistogramVec // route, method, status
	AIJobTotal       *prometheus.CounterVec   // type, status
	AIStepDuration   *prometheus.HistogramVec // type, provider
	AIProviderErrors *prometheus.CounterVec   // provider, kind
	AICostUSD        *prometheus.CounterVec   // provider, type, source
	QueueDepth       *prometheus.GaugeVec     // queue
	OutboxLag        prometheus.Gauge
}

// New registers all collectors on a fresh registry.
func New() *Metrics {
	r := prometheus.NewRegistry()
	m := &Metrics{
		Registry: r,
		HTTPDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "http_request_duration_seconds", Help: "HTTP request latency.",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}, []string{"route", "method", "status"}),
		AIJobTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ai_job_total", Help: "AI jobs by type and terminal status.",
		}, []string{"type", "status"}),
		AIStepDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "ai_step_duration_seconds", Help: "AI job step duration.",
			Buckets: []float64{.5, 1, 2, 5, 10, 30, 60, 120, 300, 900, 1800},
		}, []string{"type", "provider"}),
		AIProviderErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ai_provider_errors_total", Help: "Provider errors by kind.",
		}, []string{"provider", "kind"}),
		AICostUSD: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ai_cost_usd_total", Help: "AI spend in USD.",
		}, []string{"provider", "type", "source"}),
		QueueDepth: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "queue_depth", Help: "Pending tasks per queue.",
		}, []string{"queue"}),
		OutboxLag: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "outbox_lag_seconds", Help: "Age of the oldest unpublished outbox event.",
		}),
	}
	r.MustRegister(
		collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.HTTPDuration, m.AIJobTotal, m.AIStepDuration, m.AIProviderErrors, m.AICostUSD, m.QueueDepth, m.OutboxLag,
	)
	return m
}

// Handler serves the registry in Prometheus text format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{Registry: m.Registry})
}
