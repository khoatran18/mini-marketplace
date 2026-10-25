// Package metrics holds the order-service business counters (prefix mm_). They are process-wide collectors,
// incremented by the repository/service layers (so the worker paths are counted too) and registered once on
// the ops registry with Register. Labels are low-cardinality enums only.
package metrics

import (
	"order-service/pkg/model"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	CheckoutsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mm_checkouts_total", Help: "Checkout attempts by result (success, idempotent_replay, failed).",
	}, []string{"result"})
	OrdersCreatedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "mm_orders_created_total", Help: "Orders created by checkout (one per store).",
	})
	StatusTransitionsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mm_order_status_transitions_total", Help: "Order status changes written to order_status_histories, by target status.",
	}, []string{"to_status"})
	OrdersPaymentExpiredTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "mm_orders_payment_expired_total", Help: "Unpaid orders expired by the timers worker (stock reservation released).",
	})
	OrdersAutoDeliveredTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "mm_orders_auto_delivered_total", Help: "Shipped orders auto-marked delivered by the timers worker.",
	})
	CartMergesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "mm_order_cart_merges_total", Help: "Guest carts merged into the stored cart after login.",
	})
)

// Checkout result label values.
const (
	ResultSuccess = "success"
	ResultReplay  = "idempotent_replay"
	ResultFailed  = "failed"
)

// Register adds the collectors to reg (with the constant service label) and pre-creates the label series at 0.
func Register(reg prometheus.Registerer, service string) {
	w := prometheus.WrapRegistererWith(prometheus.Labels{"service": service}, reg)
	w.MustRegister(CheckoutsTotal, OrdersCreatedTotal, StatusTransitionsTotal, OrdersPaymentExpiredTotal, OrdersAutoDeliveredTotal, CartMergesTotal)
	for _, r := range []string{ResultSuccess, ResultReplay, ResultFailed} {
		CheckoutsTotal.WithLabelValues(r)
	}
	for _, s := range model.AllStatuses {
		StatusTransitionsTotal.WithLabelValues(s)
	}
}
