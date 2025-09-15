package dto

type ValidateOrderKafkaEvent struct {
	OrderID uint64 `json:"order_id"`
	Success bool   `json:"success"`
}
