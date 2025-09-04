package service

import (
	"context"
	"order-service/internal/repository"
	"order-service/internal/service/adapter"
	"order-service/pkg/dto"
	"order-service/pkg/model"

	"go.uber.org/zap"
)

type OrderService struct {
	OrderRepo *repository.OrderRepository
	ZapLogger *zap.Logger
}

func NewOrderService(repo *repository.OrderRepository, logger *zap.Logger) *OrderService {
	return &OrderService{
		OrderRepo: repo,
		ZapLogger: logger,
	}
}

func (s *OrderService) CreateOrder(ctx context.Context, input *dto.CreateOrderInput) (*dto.CreateOrderOutput, error) {
	orderModel := adapter.OrderDTOToModel(input.Order)
	if err := s.OrderRepo.CreateOrder(ctx, orderModel); err != nil {
		return nil, err
	}
	return &dto.CreateOrderOutput{
		Message: "Created Order successfully",
		Success: true,
	}, nil
}

func (s *OrderService) GetOrderByIDWithItems(ctx context.Context, input *dto.GetOrderByIDWithItemsInput) (*dto.GetOrderByIDWithItemsOutput, error) {
	orderModel, err := s.OrderRepo.GetOrderByIDWithItems(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	orderDTO := adapter.OrderModelToDTO(orderModel)
	return &dto.GetOrderByIDWithItemsOutput{
		Message: "Get Order successfully",
		Success: true,
		Order: orderDTO,
	}, nil
}

func (s *OrderService) GetOrderByIDOnly(ctx context.Context, input *dto.GetOrderByIDOnlyInput) (*dto.GetOrderByIDOnlyOutput, error) {
	orderModel, err := s.OrderRepo.GetOrderByIDOnly(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	orderDTO := adapter.OrderModelToDTO(orderModel)
	return &dto.GetOrderByIDOnlyOutput{
		Message: "Get Order successfully",
		Success: true,
		Order: orderDTO,
	}, nil
}

func (s *OrderService) GetOrdersByBuyerIDStatus(ctx context.Context, input *dto.GetOrdersByBuyerIDStatusInput) ([]*dto.GetOrdersByBuyerIDStatusOutput, error) {
	orderModels, err := s.OrderRepo.GetOrdersByBuyerIDStatus(ctx, input.BuyerID, input.Status)
	if err != nil {
		return nil, err
	}
	orderDTOs := adapter.
}
