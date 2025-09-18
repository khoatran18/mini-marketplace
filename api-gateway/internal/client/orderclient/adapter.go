package orderclient

import (
	"api-gateway/pkg/dto"
	orderpb "api-gateway/pkg/pb/orderservice"
)

func OrderItemDTOToProto(orderItem *dto.OrderItem) (*orderpb.OrderItem, error) {
	if orderItem == nil {
		return nil, nil
	}
	return &orderpb.OrderItem{
		ID:        orderItem.ID,
		Name:      orderItem.Name,
		ProductId: orderItem.ProductID,
		Quantity:  orderItem.Quantity,
		Price:     orderItem.Price,
	}, nil
}
func OrderItemProtoToDTO(orderItem *orderpb.OrderItem) (*dto.OrderItem, error) {
	if orderItem == nil {
		return nil, nil
	}
	return &dto.OrderItem{
		ID:        orderItem.ID,
		Name:      orderItem.Name,
		ProductID: orderItem.ProductId,
		Quantity:  orderItem.Quantity,
		Price:     orderItem.Price,
	}, nil
}

func OrderItemsDTOToProto(orderItems []*dto.OrderItem) ([]*orderpb.OrderItem, error) {
	var items []*orderpb.OrderItem
	for _, orderItem := range orderItems {
		item, err := OrderItemDTOToProto(orderItem)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}
func OrderItemsProtoToDTO(orderItems []*orderpb.OrderItem) ([]*dto.OrderItem, error) {
	var items []*dto.OrderItem
	for _, orderItem := range orderItems {
		item, err := OrderItemProtoToDTO(orderItem)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func OrderDTOToProto(order *dto.Order) (*orderpb.Order, error) {
	orderItems, err := OrderItemsDTOToProto(order.OrderItems)
	if err != nil {
		return nil, err
	}
	return &orderpb.Order{
		Id:         order.ID,
		BuyerId:    order.BuyerID,
		Status:     order.Status,
		TotalPrice: order.TotalPrice,
		OrderItem:  orderItems,
	}, nil
}
func OrderProtoToDTO(order *orderpb.Order) (*dto.Order, error) {
	orderItems, err := OrderItemsProtoToDTO(order.GetOrderItem())
	if err != nil {
		return nil, err
	}
	return &dto.Order{
		ID:         order.GetId(),
		BuyerID:    order.GetBuyerId(),
		Status:     order.GetStatus(),
		TotalPrice: order.GetTotalPrice(),
		OrderItems: orderItems,
	}, nil
}

func OrdersDTOToProto(orders []*dto.Order) ([]*orderpb.Order, error) {
	var items []*orderpb.Order
	for _, order := range orders {
		item, err := OrderDTOToProto(order)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}
func OrdersProtoToDTO(orders []*orderpb.Order) ([]*dto.Order, error) {
	var items []*dto.Order
	for _, order := range orders {
		item, err := OrderProtoToDTO(order)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func CreateOrderInputToRequest(input *dto.CreateOrderInput) (*orderpb.CreateOrderRequest, error) {
	order, err := OrderDTOToProto(input.Order)
	if err != nil {
		return nil, err
	}
	return &orderpb.CreateOrderRequest{
		Order: order,
	}, nil
}
func CreateOrderResponseToOutput(res *orderpb.CreateOrderResponse) (*dto.CreateOrderOutput, error) {
	return &dto.CreateOrderOutput{
		Message: res.GetMessage(),
		Success: res.GetSuccess(),
	}, nil
}

func GetOrderByIDInputToRequest(input *dto.GetOrderByIDInput) (*orderpb.GetOrderByIDRequest, error) {
	return &orderpb.GetOrderByIDRequest{
		Id: input.ID,
	}, nil
}
func GetOrderByIDResponseToOutput(res *orderpb.GetOrderByIDResponse) (*dto.GetOrderByIDOutput, error) {
	order, err := OrderProtoToDTO(res.GetOrder())
	if err != nil {
		return nil, err
	}
	return &dto.GetOrderByIDOutput{
		Message: res.GetMessage(),
		Success: res.GetSuccess(),
		Order:   order,
	}, nil
}

func GetOrdersByBuyerIDStatusInputToRequest(input *dto.GetOrdersByBuyerIDStatusInput) (*orderpb.GetOrdersByBuyerIDStatusRequest, error) {
	return &orderpb.GetOrdersByBuyerIDStatusRequest{
		BuyerId: input.BuyerID,
		Status:  input.Status,
	}, nil
}
func GetOrdersByBuyerIDStatusResponseToOutput(res *orderpb.GetOrdersByBuyerIDStatusResponse) (*dto.GetOrdersByBuyerIDStatusOutput, error) {
	orders, err := OrdersProtoToDTO(res.GetOrder())
	if err != nil {
		return nil, err
	}
	return &dto.GetOrdersByBuyerIDStatusOutput{
		Message: res.GetMessage(),
		Success: res.GetSuccess(),
		Orders:  orders,
	}, nil
}

func UpdateOrderByIDInputToRequest(input *dto.UpdateOrderByIDInput) (*orderpb.UpdateOrderByIDRequest, error) {
	order, err := OrderDTOToProto(input.Order)
	if err != nil {
		return nil, err
	}
	return &orderpb.UpdateOrderByIDRequest{
		Order: order,
	}, nil
}
func UpdateOrderByIDResponseToOutput(res *orderpb.UpdateOrderByIDResponse) (*dto.UpdateOrderByIDOutput, error) {
	return &dto.UpdateOrderByIDOutput{
		Message: res.GetMassage(),
		Success: res.GetSuccess(),
	}, nil
}

func CancelOrderByIDInputToRequest(input *dto.CancelOrderByIDInput) (*orderpb.CancelOrderByIDRequest, error) {
	return &orderpb.CancelOrderByIDRequest{
		Id: input.ID,
	}, nil
}
func CancelOrderByIDResponseToOutput(res *orderpb.CancelOrderByIDResponse) (*dto.CancelOrderByIDOutput, error) {
	return &dto.CancelOrderByIDOutput{
		Message: res.GetMessage(),
		Success: res.GetSuccess(),
	}, nil
}
