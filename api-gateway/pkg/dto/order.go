package dto

type Order struct {
	ID         uint64       `json:"id"`
	BuyerID    uint64       `json:"buyer_id"`
	Status     string       `json:"status"`
	TotalPrice float64      `json:"total_price"`
	OrderItems []*OrderItem `json:"order_items"`
}

type OrderItem struct {
	ID        uint64  `json:"id"`
	Name      string  `json:"name"`
	ProductID uint64  `json:"product_id"`
	Quantity  int64   `json:"quantity"`
	Price     float64 `json:"price"`
}

type CreateOrderInput struct {
	Order *Order `json:"order"`
}
type CreateOrderOutput struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

type GetOrderByIDInput struct {
	ID uint64 `json:"order_id"`
}
type GetOrderByIDOutput struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
	Order   *Order `json:"order"`
}

type GetOrdersByBuyerIDStatusInput struct {
	BuyerID uint64 `json:"buyer_id"`
	Status  string `json:"status"`
}
type GetOrdersByBuyerIDStatusOutput struct {
	Message string   `json:"message"`
	Success bool     `json:"success"`
	Orders  []*Order `json:"orders"`
}

type UpdateOrderByIDInput struct {
	Order *Order `json:"order"`
}
type UpdateOrderByIDOutput struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

type CancelOrderByIDInput struct {
	ID uint64 `json:"order_id"`
}
type CancelOrderByIDOutput struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}
