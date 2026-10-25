package outbox

import "time"

type ValidateOrderEvent struct {
	OrderID   uint64 `gorm:"primary_key"`
	Success   bool   `gorm:"notnull"`
	Status    string `gorm:"notnull;default:'PENDING';index:idx_status_created_at,priority:1"`
	Processed bool   `gorm:"notnull;default:false"`
	// Restored is set once the reserved inventory has been given back after an order cancellation.
	Restored bool `gorm:"notnull;default:false"`
	// Shipped is set once the reserved quantity has left the warehouse (order shipped).
	Shipped bool `gorm:"notnull;default:false"`
	// Returned is set once the shipped goods of a refunded (returned) order went back to available stock.
	Returned  bool      `gorm:"notnull;default:false"`
	CreatedAt time.Time `gorm:"notnull;index:idx_status_created_at,priority:2"`
}

type ValidateOrderKafkaEvent struct {
	OrderID uint64 `json:"order_id"`
	Success bool   `json:"success"`
}
