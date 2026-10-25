package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsNilSafe(t *testing.T) {
	var m *Metrics
	m.paymentCreated("card")
	m.paymentCompleted("card", "succeeded", 1, time.Now(), time.Now())
	m.refund("succeeded")
	m.webhook("accepted")
	m.Register(prometheus.NewRegistry(), "payment-service")
}

func TestMetricsCounting(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics()
	m.Register(reg, "payment-service")
	t0 := time.Now()
	m.paymentCreated("wallet")
	m.paymentCompleted("wallet", "succeeded", 150000, t0, t0.Add(4*time.Second))
	m.paymentCompleted("wallet", "failed", 0, t0, t0.Add(time.Second))
	m.refund("succeeded")
	m.webhook("late")

	if got := testutil.ToFloat64(m.created.WithLabelValues("wallet")); got != 1 {
		t.Fatalf("created = %v", got)
	}
	if got := testutil.ToFloat64(m.amount.WithLabelValues("wallet")); got != 150000 {
		t.Fatalf("amount = %v (only successes count)", got)
	}
	if got := testutil.ToFloat64(m.completed.WithLabelValues("wallet", "failed")); got != 1 {
		t.Fatalf("failed = %v", got)
	}
	if got := testutil.ToFloat64(m.refunds.WithLabelValues("succeeded")); got != 1 {
		t.Fatalf("refunds = %v", got)
	}
	if n, err := testutil.GatherAndCount(reg, "mm_payments_created_total", "mm_payments_completed_total", "mm_payment_amount_minor_total",
		"mm_refunds_total", "mm_payment_webhooks_total", "mm_payment_processing_seconds"); err != nil || n == 0 {
		t.Fatalf("gather: %d %v", n, err)
	}
}

func TestWebhookInvalidSignatureCounted(t *testing.T) {
	m := NewMetrics()
	s := &PaymentService{Cfg: Config{WebhookSecret: testSecret, WebhookMaxSkew: time.Minute}, Now: time.Now, Metrics: m}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/payments/webhook", strings.NewReader(`{}`))
	req.Header.Set("X-Signature", "t=1,v1=bad")
	s.WebhookHandler()(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := testutil.ToFloat64(m.webhooks.WithLabelValues("invalid_signature")); got != 1 {
		t.Fatalf("invalid_signature = %v", got)
	}

	rec = httptest.NewRecorder()
	body := []byte(`{"provider_event_id":""}`)
	req = httptest.NewRequest(http.MethodPost, "/internal/payments/webhook", strings.NewReader(string(body)))
	req.Header.Set("X-Signature", Sign(testSecret, time.Now(), body))
	s.WebhookHandler()(rec, req)
	if got := testutil.ToFloat64(m.webhooks.WithLabelValues("error")); got != 1 {
		t.Fatalf("error = %v", got)
	}
}
