package adapter

import (
	"order-service/pkg/dto"
	"order-service/pkg/model"
)

// Core adapter

func OrderDTOToModel(order *dto.Order) *model.Order {
	orderItems := OrderItemsDTOToModel(order.OrderItems)
	return &model.Order{
		ID:         order.ID,
		BuyerID:    order.BuyerID,
		Status:     order.Status,
		TotalPrice: order.TotalPrice,
		OrderItems: orderItems,
		CreatedAt:  order.CreatedAt,
		UpdatedAt:  order.UpdatedAt,
	}
}
func OrderModelToDTO(order *model.Order) *dto.Order {
	orderItems := OrderItemsModelToDTO(order.OrderItems)
	return &dto.Order{
		ID:         order.ID,
		BuyerID:    order.BuyerID,
		Status:     order.Status,
		TotalPrice: order.TotalPrice,
		OrderItems: orderItems,
		CreatedAt:  order.CreatedAt,
		UpdatedAt:  order.UpdatedAt,
	}
}

func OrdersDTOToModel(orders []*dto.Order) []*model.Order {
	var orderModels []*model.Order
	for _, order := range orders {
		orderModel :=
	}
}

func OrderItemDTOToModel(orderItem *dto.OrderItem) *model.OrderItem {
	return &model.OrderItem{
		ID:        orderItem.ID,
		OrderID:   orderItem.OrderID,
		ProductID: orderItem.ProductID,
		Quantity:  orderItem.Quantity,
		Price:     orderItem.Price,
		Status:    orderItem.Status,
		CreatedAt: orderItem.CreatedAt,
		UpdatedAt: orderItem.UpdatedAt,
	}
}
func OrderItemModelToDTO(orderItem *model.OrderItem) *dto.OrderItem{
	return &dto.OrderItem{
		ID:        orderItem.ID,
		OrderID:   orderItem.OrderID,
		ProductID: orderItem.ProductID,
		Quantity:  orderItem.Quantity,
		Price:     orderItem.Price,
		Status:    orderItem.Status,
		CreatedAt: orderItem.CreatedAt,
		UpdatedAt: orderItem.UpdatedAt,
	}
}

func OrderItemsModelToDTO(orderItems []*model.OrderItem) []*dto.OrderItem {
	var orderItemDTOs []*dto.OrderItem
	for _, orderItem := range orderItems {
		orderItem := OrderItemModelToDTO(orderItem)
		orderItemDTOs = append(orderItemDTOs, orderItem)
	}
	return orderItemDTOs
}
func OrderItemsDTOToModel(orderItems []*dto.OrderItem) []*model.OrderItem {
	var orderItemDTOs []*model.OrderItem
	for _, orderItem := range orderItems {
		orderItem := OrderItemDTOToModel(orderItem)
		orderItemDTOs = append(orderItemDTOs, orderItem)
	}
	return orderItemDTOs
}
