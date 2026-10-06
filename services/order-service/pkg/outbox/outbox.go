package outbox

import "gorm.io/datatypes"

type CreateOrderEvent struct {
	OrderID uint64 `gorm:"primary_key"`
	Items   datatypes.JSON
	Status  string `gorm:"index:idx_co_kafka"`
}

// CancelOrderEvent asks product-service to release the inventory of a canceled order.
type CancelOrderEvent struct {
	OrderID uint64 `gorm:"primary_key"`
	Items   datatypes.JSON
	Status  string `gorm:"index:idx_cancel_kafka"`
}

type ItemEvent struct {
	ProductID uint64 `json:"product_id"`
	Quantity  int64  `json:"quantity"`
}

type CreateOrderKafkaEvent struct {
	OrderID uint64       `json:"order_id"`
	Items   []*ItemEvent `json:"items"`
}

type CancelOrderKafkaEvent struct {
	OrderID uint64       `json:"order_id"`
	Items   []*ItemEvent `json:"items"`
}
