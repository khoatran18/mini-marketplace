package outbox

import "gorm.io/datatypes"

type CreateOrderEvent struct {
	OrderID uint64 `gorm:"primary_key"`
	Items   datatypes.JSON
	Status  string `gorm:"index:idx_co_kafka"`
}
type ItemEvent struct {
	ProductID uint64 `json:"product_id"`
	Quantity  int64  `json:"quantity"`
}

type CreateOrderKafkaEvent struct {
	OrderID uint64       `json:"order_id"`
	Items   []*ItemEvent `json:"items"`
}

//type ItemKafkaEvent struct {
//	ProductID string `json:"product_id"`
//	Quantity  int    `json:"quantity"`
//}
