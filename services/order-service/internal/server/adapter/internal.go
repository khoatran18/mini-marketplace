package adapter

import (
	"order-service/pkg/dto"
	orderpb "order-service/pkg/pb"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func OrderProtoToDTO(order *orderpb.Order) (*dto.Order, error) {
	orderItemsDTO, err := OrderItemsProtoToDTO(order.GetOrderItem())
	if err != nil {
		return nil, err
	}
	return &dto.Order{
		ID:         order.GetId(),
		BuyerID:    order.GetBuyerId(),
		Status:     order.GetStatus(),
		TotalPrice: order.GetTotalPrice(),
		OrderItems: orderItemsDTO,
		CreatedAt:  order.GetCreatedAt().AsTime(),
		UpdatedAt:  order.GetUpdatedAt().AsTime(),
	}, nil
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
		CreatedAt:  timestamppb.New(order.CreatedAt),
		UpdatedAt:  timestamppb.New(order.UpdatedAt),
	}, nil
}

func OrdersProtoToDTO(orders []*orderpb.Order) ([]*dto.Order, error) {
	var dtoOrders []*dto.Order
	for _, order := range orders {
		orderDTO, err := OrderProtoToDTO(order)
		if err != nil {
			return nil, err
		}
		dtoOrders = append(dtoOrders, orderDTO)
	}
	return dtoOrders, nil
}
func OrdersDTOToProto(orders []*dto.Order) ([]*orderpb.Order, error) {
	var ordersProto []*orderpb.Order
	for _, order := range orders {
		orderProto, err := OrderDTOToProto(order)
		if err != nil {
			return nil, err
		}
		ordersProto = append(ordersProto, orderProto)
	}
	return ordersProto, nil
}

func OrderItemProtoToDTO(orderItem *orderpb.OrderItem) (*dto.OrderItem, error) {
	return &dto.OrderItem{
		ID:        orderItem.GetID(),
		Name:      orderItem.GetName(),
		OrderID:   orderItem.GetOrderId(),
		ProductID: orderItem.GetProductId(),
		Quantity:  orderItem.GetQuantity(),
		Price:     orderItem.GetPrice(),
		Status:    orderItem.GetStatus(),
		CreatedAt: orderItem.GetCreatedAt().AsTime(),
		UpdatedAt: orderItem.GetUpdatedAt().AsTime(),
	}, nil
}
func OrderItemDTOToProto(orderItem *dto.OrderItem) (*orderpb.OrderItem, error) {
	return &orderpb.OrderItem{
		ID:        orderItem.ID,
		Name:      orderItem.Name,
		OrderId:   orderItem.OrderID,
		ProductId: orderItem.ProductID,
		Quantity:  orderItem.Quantity,
		Price:     orderItem.Price,
		Status:    orderItem.Status,
		CreatedAt: timestamppb.New(orderItem.CreatedAt),
		UpdatedAt: timestamppb.New(orderItem.UpdatedAt),
	}, nil
}

func OrderItemsProtoToDTO(orderItems []*orderpb.OrderItem) ([]*dto.OrderItem, error) {
	var orderItemsDTO []*dto.OrderItem
	for _, orderItem := range orderItems {
		dto, err := OrderItemProtoToDTO(orderItem)
		if err != nil {
			return nil, err
		}
		orderItemsDTO = append(orderItemsDTO, dto)
	}
	return orderItemsDTO, nil
}
func OrderItemsDTOToProto(orderItems []*dto.OrderItem) ([]*orderpb.OrderItem, error) {
	var orderItemsDTO []*orderpb.OrderItem
	for _, orderItem := range orderItems {
		orderItem, err := OrderItemDTOToProto(orderItem)
		if err != nil {
			return nil, err
		}
		orderItemsDTO = append(orderItemsDTO, orderItem)
	}
	return orderItemsDTO, nil
}

func CreOrdRequestToInput(req *orderpb.CreateOrderRequest) (*dto.CreateOrderInput, error) {
	orderDTO, err := OrderProtoToDTO(req.Order)
	if err != nil {
		return nil, err
	}
	return &dto.CreateOrderInput{
		Order: orderDTO,
	}, nil
}

func CreOrdOutputToResponse(output *dto.CreateOrderOutput) (*orderpb.CreateOrderResponse, error) {
	return &orderpb.CreateOrderResponse{
		Message: output.Message,
		Success: output.Success,
	}, nil
}

func GetOrdByIDRequestToInput(req *orderpb.GetOrderByIDRequest) (*dto.GetOrderByIDInput, error) {
	return &dto.GetOrderByIDInput{
		ID: req.GetId(),
	}, nil
}
func GetOrdByIDOutputToResponse(output *dto.GetOrderByIDOutput) (*orderpb.GetOrderByIDResponse, error) {
	orderProto, err := OrderDTOToProto(output.Order)
	if err != nil {
		return nil, err
	}
	return &orderpb.GetOrderByIDResponse{
		Message: output.Message,
		Success: output.Success,
		Order:   orderProto,
	}, nil
}

func GetOrdsByBuyIDStaRequestToInput(req *orderpb.GetOrdersByBuyerIDStatusRequest) (*dto.GetOrdersByBuyerIDStatusInput, error) {
	return &dto.GetOrdersByBuyerIDStatusInput{
		BuyerID: req.GetBuyerId(),
		Status:  req.GetStatus(),
	}, nil
}
func GetOrdsByBuyIDStaOutputToResponse(output *dto.GetOrdersByBuyerIDStatusOutput) (*orderpb.GetOrdersByBuyerIDStatusResponse, error) {
	ordersProto, err := OrdersDTOToProto(output.Orders)
	if err != nil {
		return nil, err
	}
	return &orderpb.GetOrdersByBuyerIDStatusResponse{
		Message: output.Message,
		Success: output.Success,
		Order:   ordersProto,
	}, nil
}

func GetOrdItesByOrdIDRequestToInput(req *orderpb.GetOrderItemsByOrderIDRequest) (*dto.GetOrderItemsByOrderIDInput, error) {
	return &dto.GetOrderItemsByOrderIDInput{
		OrderID: req.GetOrderId(),
	}, nil
}
func GetOrdItesByOrdIDOutputToResponse(output *dto.GetOrderItemsByOrderIDOutput) (*orderpb.GetOrderItemsByOrderIDResponse, error) {
	orderItemsProto, err := OrderItemsDTOToProto(output.OrderItems)
	if err != nil {
		return nil, err
	}

	return &orderpb.GetOrderItemsByOrderIDResponse{
		Message:   output.Message,
		Success:   output.Success,
		OrderItem: orderItemsProto,
	}, nil
}

func UpdOrdByIDRequestToInput(req *orderpb.UpdateOrderByIDRequest) (*dto.UpdateOrderByIDInput, error) {
	orderDTO, err := OrderProtoToDTO(req.GetOrder())
	if err != nil {
		return nil, err
	}

	return &dto.UpdateOrderByIDInput{
		Order: orderDTO,
	}, nil
}
func UpdOrdByIDOutputToResponse(output *dto.UpdateOrderByIDOutput) (*orderpb.UpdateOrderByIDResponse, error) {
	return &orderpb.UpdateOrderByIDResponse{
		Massage: output.Message,
		Success: output.Success,
	}, nil
}

func CanOrdByIDRequestToInput(req *orderpb.CancelOrderByIDRequest) (*dto.CancelOrderByIDInput, error) {
	return &dto.CancelOrderByIDInput{
		ID: req.GetId(),
	}, nil
}
func CanOrdByIDOutputToResponse(output *dto.CancelOrderByIDOutput) (*orderpb.CancelOrderByIDResponse, error) {
	return &orderpb.CancelOrderByIDResponse{
		Message: output.Message,
		Success: output.Success,
	}, nil
}
