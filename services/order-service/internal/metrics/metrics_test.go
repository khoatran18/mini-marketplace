package metrics

import (
	"strconv"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRegisterAndCount(t *testing.T) {
	reg := prometheus.NewRegistry()
	Register(reg, "order-service")

	CheckoutsTotal.WithLabelValues(ResultReplay).Inc()
	OrdersCreatedTotal.Add(2)
	StatusTransitionsTotal.WithLabelValues("PAID").Inc()
	OrdersPaymentExpiredTotal.Inc()
	OrdersAutoDeliveredTotal.Inc()
	CartMergesTotal.Inc()

	if got := testutil.ToFloat64(CheckoutsTotal.WithLabelValues(ResultReplay)); got < 1 {
		t.Fatalf("replay=%v", got)
	}
	want := `
# HELP mm_orders_payment_expired_total Unpaid orders expired by the timers worker (stock reservation released).
# TYPE mm_orders_payment_expired_total counter
mm_orders_payment_expired_total{service="order-service"} ` + trimFloat(testutil.ToFloat64(OrdersPaymentExpiredTotal)) + "\n"
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want), "mm_orders_payment_expired_total"); err != nil {
		t.Fatal(err)
	}
	n, err := testutil.GatherAndCount(reg, "mm_checkouts_total", "mm_order_status_transitions_total", "mm_orders_created_total",
		"mm_orders_auto_delivered_total", "mm_order_cart_merges_total")
	if err != nil || n < 3+11+3 {
		t.Fatalf("series=%d err=%v", n, err)
	}
}

func trimFloat(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
