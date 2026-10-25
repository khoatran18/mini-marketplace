package ops

import "github.com/prometheus/client_golang/prometheus"

// Domain metrics of product-service. Package-level so repositories can record without holding the Server;
// New registers them (with the service label) on the Server registry.
var (
	inventoryReservations = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mm_inventory_reservations_total", Help: "Inventory reservation outcomes."}, []string{"result"})
	inventoryAdjustments  = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mm_inventory_adjustments_total", Help: "Inventory ledger entries by reason."}, []string{"reason"})
	productsChanged       = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mm_products_changed_total", Help: "Catalog changes."}, []string{"op"})
)

var (
	reservationResults = map[string]bool{"reserved": true, "insufficient": true, "released": true, "committed": true}
	adjustmentReasons  = map[string]bool{"initial": true, "reserve": true, "release": true, "sale": true, "restock": true, "adjust": true, "update": true}
	productOps         = map[string]bool{"create": true, "update": true, "delete": true}
)

func init() {
	// pre-create the known series so dashboards see zeros
	for r := range reservationResults {
		inventoryReservations.WithLabelValues(r)
	}
	for r := range adjustmentReasons {
		inventoryAdjustments.WithLabelValues(r)
	}
	for o := range productOps {
		productsChanged.WithLabelValues(o)
	}
}

// IncReservation counts a reservation outcome: reserved|insufficient|released|committed. Unknown values are ignored.
func IncReservation(result string) {
	if reservationResults[result] {
		inventoryReservations.WithLabelValues(result).Inc()
	}
}

// IncAdjustment counts an inventory ledger entry; reason must be one of the ledger reasons, otherwise it is "adjust"-free ignored.
func IncAdjustment(reason string) {
	if adjustmentReasons[reason] {
		inventoryAdjustments.WithLabelValues(reason).Inc()
	}
}

// IncProductChanged counts a catalog change: create|update|delete.
func IncProductChanged(op string) {
	if productOps[op] {
		productsChanged.WithLabelValues(op).Inc()
	}
}

func registerDomain(r prometheus.Registerer) {
	r.MustRegister(inventoryReservations, inventoryAdjustments, productsChanged)
}
