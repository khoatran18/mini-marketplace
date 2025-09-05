package dto

import (
	"time"
)

type Order struct {
	ID         uint64
	BuyerID    int
	Status     string
	TotalPrice int
	OrderItems []*OrderItem
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type OrderItem struct {
	ID        uint64
	Name      string
	OrderID   uint64
	ProductID int
	Quantity  int
	Price     int
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

type GetOrderByIDWithItemsInput struct {
	ID uint64
}
type GetOrderByIDWithItemsOutput struct {
	Message string
	Success bool
	Order   *Order
}

type GetOrderByIDOnlyInput struct {
	ID uint64
}
type GetOrderByIDOnlyOutput struct {
	Message string
	Success bool
	Order   *Order
}

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
