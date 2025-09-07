package adapter

import (
	"maps"
	"order-service/internal/client/productclient"
	"order-service/pkg/dto"
	"order-service/pkg/model"
	"slices"
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
func OrderModelToDTO(order *model.Order, products []*productclient.ProductDTOClient) *dto.Order {
	mapName := MapOrderItemIDToName(products)
	orderItems := OrderItemsModelToDTO(order.OrderItems, mapName)
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
		orderModel := OrderDTOToModel(order)
		orderModels = append(orderModels, orderModel)
	}
	return orderModels
}
func OrdersModelToDTO(orders []*model.Order, products []*productclient.ProductDTOClient) []*dto.Order {
	var orderModels []*dto.Order
	for _, order := range orders {
		orderModel := OrderModelToDTO(order, products)
		orderModels = append(orderModels, orderModel)
	}
	return orderModels
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
func OrderItemModelToDTO(orderItem *model.OrderItem, name string) *dto.OrderItem {
	return &dto.OrderItem{
		ID:        orderItem.ID,
		Name:      name,
		OrderID:   orderItem.OrderID,
		ProductID: orderItem.ProductID,
		Quantity:  orderItem.Quantity,
		Price:     orderItem.Price,
		Status:    orderItem.Status,
		CreatedAt: orderItem.CreatedAt,
		UpdatedAt: orderItem.UpdatedAt,
	}
}

func OrderItemsModelToDTO(orderItems []*model.OrderItem, mapName map[uint64]string) []*dto.OrderItem {
	var orderItemDTOs []*dto.OrderItem
	for _, orderItem := range orderItems {
		orderItem := OrderItemModelToDTO(orderItem, mapName[orderItem.ID])
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

func MapOrderItemIDToName(products []*productclient.ProductDTOClient) map[uint64]string {
	orderItemMap := map[uint64]string{}
	for _, product := range products {
		orderItemMap[product.ID] = product.Name
	}
	return orderItemMap
}

func FilterItemIDsByOrder(orders []*model.Order) []uint64 {
	idsMap := map[uint64]struct{}{}
	for _, order := range orders {
		for _, orderItem := range order.OrderItems {
			idsMap[orderItem.ID] = struct{}{}
		}
	}
	ids := slices.Collect(maps.Keys(idsMap))
	slices.Sort(ids)
	return ids
}

func FilterItemIDsByItems(orderItems []*model.OrderItem) []uint64 {
	idsMap := map[uint64]struct{}{}
	for _, order := range orderItems {
		idsMap[order.ID] = struct{}{}
	}
	ids := slices.Collect(maps.Keys(idsMap))
	slices.Sort(ids)
	return ids
}
