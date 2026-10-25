package service

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics are the business metrics of payment-service (names are a contract with the Grafana dashboards).
// All methods are safe on a nil *Metrics, so tests and tools can run without metrics.
type Metrics struct {
	created    *prometheus.CounterVec
	completed  *prometheus.CounterVec
	amount     *prometheus.CounterVec
	refunds    *prometheus.CounterVec
	webhooks   *prometheus.CounterVec
	processing prometheus.Histogram
}

var (
	metricMethods  = []string{"card", "wallet", "transfer"}
	metricResults  = []string{"succeeded", "failed", "expired", "cancelled"}
	metricRefunds  = []string{"succeeded", "failed", "duplicate", "ignored"}
	metricWebhooks = []string{"accepted", "invalid_signature", "duplicate", "late", "error"}
)

// NewMetrics creates the collectors; call Register once the ops registry exists.
func NewMetrics() *Metrics {
	m := &Metrics{
		created:   prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mm_payments_created_total", Help: "Payments created, by method."}, []string{"method"}),
		completed: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mm_payments_completed_total", Help: "Payments that reached an end state, by method and result."}, []string{"method", "result"}),
		amount:    prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mm_payment_amount_minor_total", Help: "Sum of successfully paid amounts in minor units (1/100 VND), by method."}, []string{"method"}),
		refunds:   prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mm_refunds_total", Help: "Refund requests, by result."}, []string{"result"}),
		webhooks:  prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mm_payment_webhooks_total", Help: "Provider webhooks received, by result."}, []string{"result"}),
		processing: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "mm_payment_processing_seconds", Help: "Time from payment creation to its end state.",
			Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600, 900, 1800, 3600},
		}),
	}
	for _, me := range metricMethods {
		m.created.WithLabelValues(me)
		m.amount.WithLabelValues(me)
		for _, r := range metricResults {
			m.completed.WithLabelValues(me, r)
		}
	}
	for _, r := range metricRefunds {
		m.refunds.WithLabelValues(r)
	}
	for _, r := range metricWebhooks {
		m.webhooks.WithLabelValues(r)
	}
	return m
}

// Register adds the collectors to reg with the constant label service=<service> (like every other mm_ metric).
func (m *Metrics) Register(reg prometheus.Registerer, service string) {
	if m == nil {
		return
	}
	w := prometheus.WrapRegistererWith(prometheus.Labels{"service": service}, reg)
	w.MustRegister(m.created, m.completed, m.amount, m.refunds, m.webhooks, m.processing)
}

func (m *Metrics) paymentCreated(method string) {
	if m != nil {
		m.created.WithLabelValues(method).Inc()
	}
}

// paymentCompleted counts an end state; result is succeeded|failed|expired|cancelled. created is the payment's creation time.
func (m *Metrics) paymentCompleted(method, result string, amountMinor int64, created, now time.Time) {
	if m == nil {
		return
	}
	m.completed.WithLabelValues(method, result).Inc()
	if result == "succeeded" && amountMinor > 0 {
		m.amount.WithLabelValues(method).Add(float64(amountMinor))
	}
	if !created.IsZero() && now.After(created) {
		m.processing.Observe(now.Sub(created).Seconds())
	}
}

func (m *Metrics) refund(result string) {
	if m != nil {
		m.refunds.WithLabelValues(result).Inc()
	}
}

func (m *Metrics) webhook(result string) {
	if m != nil {
		m.webhooks.WithLabelValues(result).Inc()
	}
}
