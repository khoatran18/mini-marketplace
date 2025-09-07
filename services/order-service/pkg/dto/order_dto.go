package dto

import (
	"time"
)

type Order struct {
	ID         uint64
	BuyerID    uint64
	Status     string
	TotalPrice float64
	OrderItems []*OrderItem
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type OrderItem struct {
	ID        uint64
	Name      string
	OrderID   uint64
	ProductID uint64
	Quantity  int64
	Price     float64
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CreateOrderInput struct {
	Order *Order
}
type CreateOrderOutput struct {
	Message string
	Success bool
}

type GetOrderByIDInput struct {
	ID uint64
}
type GetOrderByIDOutput struct {
	Message string
	Success bool
	Order   *Order
}

//type GetOrderByIDOnlyInput struct {
//	ID uint64
//}
//type GetOrderByIDOnlyOutput struct {
//	Message string
//	Success bool
//	Order   *Order
//}

type GetOrdersByBuyerIDStatusInput struct {
	BuyerID uint64
	Status  string
}
type GetOrdersByBuyerIDStatusOutput struct {
	Message string
	Success bool
	Orders  []*Order
}

type GetOrderItemsByOrderIDInput struct {
	OrderID uint64
}
type GetOrderItemsByOrderIDOutput struct {
	Message    string
	Success    bool
	OrderItems []*OrderItem
}

type UpdateOrderByIDInput struct {
	Order *Order
}
type UpdateOrderByIDOutput struct {
	Message string
	Success bool
}

type UpdateOrderItemsByIDInput struct {
	OrderItems []*OrderItem
}
type UpdateOrderItemsByIDOutput struct {
	Message string
	Success bool
}

type CancelOrderByIDInput struct {
	ID uint64
}
type CancelOrderByIDOutput struct {
	Message string
	Success bool
}
