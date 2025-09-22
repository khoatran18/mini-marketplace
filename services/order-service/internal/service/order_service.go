package service

import (
	"context"
	"encoding/json"
	"fmt"
	"order-service/internal/client/productclient"
	"order-service/internal/client/serviceclientmanager"
	"order-service/internal/config/messagequeue"
	"order-service/internal/config/messagequeue/kafkaimpl"
	"order-service/internal/repository"
	"order-service/internal/service/adapter"
	"order-service/pkg/dto"
	"order-service/pkg/model"
	"order-service/pkg/outbox"

	"go.uber.org/zap"
)

type OrderService struct {
	OrderRepo   *repository.OrderRepository
	ZapLogger   *zap.Logger
	MQProducer  messagequeue.Producer
	MQConsumer  messagequeue.Consumer
	KafkaClient *kafkaimpl.KafkaClient
	SCM         *serviceclientmanager.ServiceClientManager
}

func NewOrderService(repo *repository.OrderRepository, logger *zap.Logger, scm *serviceclientmanager.ServiceClientManager,
	producer messagequeue.Producer, consumer messagequeue.Consumer, kafkaClient *kafkaimpl.KafkaClient) *OrderService {
	return &OrderService{
		OrderRepo:   repo,
		ZapLogger:   logger,
		MQProducer:  producer,
		MQConsumer:  consumer,
		KafkaClient: kafkaClient,
		SCM:         scm,
	}
}

func (s *OrderService) CreateOrder(ctx context.Context, input *dto.CreateOrderInput) (*dto.CreateOrderOutput, error) {
	orderModel := adapter.OrderDTOToModel(input.Order)

	// Create OutboxModel
	items := orderModel.OrderItems
	var outboxItems []*outbox.ItemEvent
	for _, item := range items {
		outboxItem := &outbox.ItemEvent{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
		}
		outboxItems = append(outboxItems, outboxItem)
	}
	outboxItemsDTO, err := json.Marshal(outboxItems)
	if err != nil {
		return nil, err
	}
	createOrderEvent := &outbox.CreateOrderEvent{
		OrderID: orderModel.ID,
		Items:   outboxItemsDTO,
		Status:  "PENDING",
	}

	if err := s.OrderRepo.CreateOrder(ctx, orderModel, createOrderEvent); err != nil {
		return nil, err
	}
	return &dto.CreateOrderOutput{
		Message: "Created Order successfully",
		Success: true,
	}, nil
}

func (s *OrderService) GetOrderByID(ctx context.Context, input *dto.GetOrderByIDInput) (*dto.GetOrderByIDOutput, error) {
	orderModel, err := s.OrderRepo.GetOrderByID(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	itemsID := adapter.FilterItemIDsByOrder([]*model.Order{orderModel})
	clientProductOutput, err := s.SCM.ProductServiceClient.GetProductsByID(ctx, &productclient.GetProductsByIDInput{
		IDs: itemsID,
	})
	if err != nil {
		return nil, err
	}

	orderDTO := adapter.OrderModelToDTO(orderModel, clientProductOutput.Products)
	return &dto.GetOrderByIDOutput{
		Message: "Get Order successfully",
		Success: true,
		Order:   orderDTO,
	}, nil
}

//func (s *OrderService) GetOrderByIDOnly(ctx context.Context, input *dto.GetOrderByIDOnlyInput) (*dto.GetOrderByIDOnlyOutput, error) {
//	orderModel, err := s.OrderRepo.GetOrderByIDOnly(ctx, input.ID)
//	if err != nil {
//		return nil, err
//	}
//
//	itemsID := adapter.FilterItemIDsByOrder([]*model.Order{orderModel})
//	clientProductOutput, err := s.SCM.ProductServiceClient.GetProductsByID(ctx, &productclient.GetProductsByIDInput{
//		IDs: itemsID,
//	})
//	if err != nil {
//		return nil, err
//	}
//	orderDTO := adapter.OrderModelToDTO(orderModel, clientProductOutput.Products)
//	return &dto.GetOrderByIDOnlyOutput{
//		Message: "Get Order successfully",
//		Success: true,
//		Order:   orderDTO,
//	}, nil
//}

func (s *OrderService) GetOrdersByBuyerIDStatus(ctx context.Context, input *dto.GetOrdersByBuyerIDStatusInput) (*dto.GetOrdersByBuyerIDStatusOutput, error) {
	orderModels, err := s.OrderRepo.GetOrdersByBuyerIDStatus(ctx, input.BuyerID, input.Status)
	fmt.Println("Status before: ", input.Status)
	if err != nil {
		return nil, err
	}

	itemsID := adapter.FilterItemIDsByOrder(orderModels)
	productClientOutput, err := s.SCM.ProductServiceClient.GetProductsByID(ctx, &productclient.GetProductsByIDInput{
		IDs: itemsID,
	})
	if err != nil {
		return nil, err
	}

	orderDTOs := adapter.OrdersModelToDTO(orderModels, productClientOutput.Products)
	fmt.Println(orderDTOs)
	return &dto.GetOrdersByBuyerIDStatusOutput{
		Message: "Get Order successfully",
		Success: true,
		Orders:  orderDTOs,
	}, nil
}

func (s *OrderService) GetOrderItemsByOrderID(ctx context.Context, input *dto.GetOrderItemsByOrderIDInput) (*dto.GetOrderItemsByOrderIDOutput, error) {
	orderItemsModel, err := s.OrderRepo.GetOrderItemsByOrderID(ctx, input.OrderID)
	if err != nil {
		return nil, err
	}
	ids := adapter.FilterItemIDsByItems(orderItemsModel)
	clientProductOutput, err := s.SCM.ProductServiceClient.GetProductsByID(ctx, &productclient.GetProductsByIDInput{
		IDs: ids,
	})

	mapName := adapter.MapOrderItemIDToName(clientProductOutput.Products)
	orderItemsDTO := adapter.OrderItemsModelToDTO(orderItemsModel, mapName)
	return &dto.GetOrderItemsByOrderIDOutput{
		Message:    "Get Order Items successfully",
		Success:    true,
		OrderItems: orderItemsDTO,
	}, nil
}

func (s *OrderService) UpdateOrderByID(ctx context.Context, input *dto.UpdateOrderByIDInput) (*dto.UpdateOrderByIDOutput, error) {
	fmt.Printf("ID: %v", input.Order.ID)
	orderModel := adapter.OrderDTOToModel(input.Order)
	if err := s.OrderRepo.UpdateOrderByID(ctx, orderModel); err != nil {
		return nil, err
	}
	return &dto.UpdateOrderByIDOutput{
		Message: "Update Order successfully",
		Success: true,
	}, nil
}

func (s *OrderService) CancelOrderByID(ctx context.Context, input *dto.CancelOrderByIDInput) (*dto.CancelOrderByIDOutput, error) {
	if err := s.OrderRepo.CancelOrderByID(ctx, input.ID); err != nil {
		return nil, err
	}
	return &dto.CancelOrderByIDOutput{
		Message: "Cancel Order successfully",
		Success: true,
	}, nil
}
