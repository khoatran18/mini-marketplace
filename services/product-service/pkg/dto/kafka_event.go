package dto

type ItemEvent struct {
	ProductID uint64 `json:"product_id"`
	Quantity  int64  `json:"quantity"`
}

type CreateOrderKafkaEvent struct {
	OrderID uint64       `json:"order_id"`
	Items   []*ItemEvent `json:"items"`
}

// CancelOrderKafkaEvent is published by order-service when a confirmed order is canceled.
type CancelOrderKafkaEvent struct {
	OrderID uint64       `json:"order_id"`
	Items   []*ItemEvent `json:"items"`
}
