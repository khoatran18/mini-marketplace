package service

import (
	"context"
	"encoding/json"
	"errors"
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
	"slices"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// maxItemQuantity guards against absurd quantities (and integer overflow when pricing).
const maxItemQuantity = 100000

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

// pricedItem is a validated order line with the authoritative price from product-service.
type pricedItem struct {
	productID uint64
	quantity  int64
	unitMinor int64
	lineMinor int64
}

// priceItems merges duplicate products, validates quantities and prices every line using
// the product catalog. Prices and totals supplied by the client are never used.
func (s *OrderService) priceItems(ctx context.Context, items []*dto.OrderItem) ([]*pricedItem, int64, error) {
	if len(items) == 0 {
		return nil, 0, status.Error(codes.InvalidArgument, "order must contain at least one item")
	}

	// Merge duplicate products
	quantities := map[uint64]int64{}
	for _, item := range items {
		if item == nil || item.ProductID == 0 {
			return nil, 0, status.Error(codes.InvalidArgument, "item product_id is required")
		}
		if item.Quantity <= 0 || item.Quantity > maxItemQuantity {
			return nil, 0, status.Errorf(codes.InvalidArgument, "quantity of product %d must be between 1 and %d", item.ProductID, maxItemQuantity)
		}
		quantities[item.ProductID] += item.Quantity
		if quantities[item.ProductID] > maxItemQuantity {
			return nil, 0, status.Errorf(codes.InvalidArgument, "quantity of product %d must be between 1 and %d", item.ProductID, maxItemQuantity)
		}
	}
	ids := make([]uint64, 0, len(quantities))
	for id := range quantities {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	out, err := s.SCM.ProductServiceClient.GetProductsByID(ctx, &productclient.GetProductsByIDInput{IDs: ids})
	if err != nil {
		s.ZapLogger.Error("OrderService: can not load products", zap.Error(err))
		return nil, 0, status.Error(codes.Unavailable, "product service is unavailable")
	}
	products := map[uint64]*productclient.ProductDTOClient{}
	for _, p := range out.Products {
		products[p.ID] = p
	}

	var total int64
	priced := make([]*pricedItem, 0, len(ids))
	for _, id := range ids {
		p, ok := products[id]
		if !ok {
			return nil, 0, status.Errorf(codes.NotFound, "product %d not found", id)
		}
		qty := quantities[id]
		// Advisory check for fast feedback; the authoritative decrement happens in product-service.
		if p.Inventory < qty {
			return nil, 0, status.Errorf(codes.FailedPrecondition, "not enough inventory for product %d", id)
		}
		unit := toMinor(p.Price)
		if unit <= 0 {
			return nil, 0, status.Errorf(codes.FailedPrecondition, "product %d is not available for sale", id)
		}
		line := unit * qty
		total += line
		priced = append(priced, &pricedItem{productID: id, quantity: qty, unitMinor: unit, lineMinor: line})
	}
	return priced, total, nil
}

func (s *OrderService) CreateOrder(ctx context.Context, input *dto.CreateOrderInput) (*dto.CreateOrderOutput, error) {
	if input == nil || input.Order == nil {
		return nil, status.Error(codes.InvalidArgument, "order is required")
	}
	if input.Order.BuyerID == 0 {
		return nil, status.Error(codes.InvalidArgument, "buyer_id is required")
	}

	priced, totalMinor, err := s.priceItems(ctx, input.Order.OrderItems)
	if err != nil {
		return nil, err
	}

	orderModel := &model.Order{
		BuyerID:    input.Order.BuyerID,
		Status:     model.StatusPending,
		TotalPrice: fromMinor(totalMinor),
	}
	var outboxItems []*outbox.ItemEvent
	for _, p := range priced {
		orderModel.OrderItems = append(orderModel.OrderItems, &model.OrderItem{
			ProductID: p.productID,
			Quantity:  p.quantity,
			Price:     fromMinor(p.unitMinor),
			Status:    "ACTIVE",
		})
		outboxItems = append(outboxItems, &outbox.ItemEvent{ProductID: p.productID, Quantity: p.quantity})
	}
	outboxItemsJSON, err := json.Marshal(outboxItems)
	if err != nil {
		return nil, err
	}

	// OrderID is filled in by the repository once the order row has been inserted.
	createOrderEvent := &outbox.CreateOrderEvent{Items: outboxItemsJSON, Status: "PENDING"}
	if err := s.OrderRepo.CreateOrder(ctx, orderModel, createOrderEvent); err != nil {
		s.ZapLogger.Error("OrderService: create order failed", zap.Error(err))
		return nil, status.Error(codes.Internal, "can not create order")
	}
	return &dto.CreateOrderOutput{
		Message: fmt.Sprintf("Created Order %d successfully", orderModel.ID),
		Success: true,
	}, nil
}

// productsForItems loads the catalog entries used to label order items with product names.
func (s *OrderService) productsForItems(ctx context.Context, ids []uint64) ([]*productclient.ProductDTOClient, error) {
	out, err := s.SCM.ProductServiceClient.GetProductsByID(ctx, &productclient.GetProductsByIDInput{IDs: ids})
	if err != nil {
		s.ZapLogger.Error("OrderService: can not load products", zap.Error(err))
		return nil, status.Error(codes.Unavailable, "product service is unavailable")
	}
	return out.Products, nil
}

func (s *OrderService) GetOrderByID(ctx context.Context, input *dto.GetOrderByIDInput) (*dto.GetOrderByIDOutput, error) {
	orderModel, err := s.OrderRepo.GetOrderByID(ctx, input.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, err
	}

	products, err := s.productsForItems(ctx, adapter.FilterItemIDsByOrder([]*model.Order{orderModel}))
	if err != nil {
		return nil, err
	}

	return &dto.GetOrderByIDOutput{
		Message: "Get Order successfully",
		Success: true,
		Order:   adapter.OrderModelToDTO(orderModel, products),
	}, nil
}

func (s *OrderService) GetOrdersByBuyerIDStatus(ctx context.Context, input *dto.GetOrdersByBuyerIDStatusInput) (*dto.GetOrdersByBuyerIDStatusOutput, error) {
	if !slices.Contains(repository.OrderStatus, input.Status) {
		return nil, status.Errorf(codes.InvalidArgument, "status must be one of %v", repository.OrderStatus)
	}
	orderModels, err := s.OrderRepo.GetOrdersByBuyerIDStatus(ctx, input.BuyerID, input.Status)
	if err != nil {
		return nil, err
	}

	products, err := s.productsForItems(ctx, adapter.FilterItemIDsByOrder(orderModels))
	if err != nil {
		return nil, err
	}

	return &dto.GetOrdersByBuyerIDStatusOutput{
		Message: "Get Order successfully",
		Success: true,
		Orders:  adapter.OrdersModelToDTO(orderModels, products),
	}, nil
}

func (s *OrderService) GetOrderItemsByOrderID(ctx context.Context, input *dto.GetOrderItemsByOrderIDInput) (*dto.GetOrderItemsByOrderIDOutput, error) {
	orderItemsModel, err := s.OrderRepo.GetOrderItemsByOrderID(ctx, input.OrderID)
	if err != nil {
		return nil, err
	}
	products, err := s.productsForItems(ctx, adapter.FilterItemIDsByItems(orderItemsModel))
	if err != nil {
		return nil, err
	}

	mapName := adapter.MapOrderItemIDToName(products)
	return &dto.GetOrderItemsByOrderIDOutput{
		Message:    "Get Order Items successfully",
		Success:    true,
		OrderItems: adapter.OrderItemsModelToDTO(orderItemsModel, mapName),
	}, nil
}

// allowedPredecessors lists, for each target status, the statuses it may be reached from.
var allowedPredecessors = map[string][]string{
	model.StatusSuccess:  {model.StatusPending},
	model.StatusFailed:   {model.StatusPending},
	model.StatusCanceled: {model.StatusSuccess},
}

// UpdateOrderByID only changes the order status, and only along the allowed transitions.
// Every other field of the input is ignored, so callers cannot rewrite buyer, items or prices.
// Canceling goes through CancelOrderByID so inventory is released.
func (s *OrderService) UpdateOrderByID(ctx context.Context, input *dto.UpdateOrderByIDInput) (*dto.UpdateOrderByIDOutput, error) {
	if input == nil || input.Order == nil || input.Order.ID == 0 {
		return nil, status.Error(codes.InvalidArgument, "order id is required")
	}
	if input.Order.Status == model.StatusCanceled {
		return nil, status.Error(codes.InvalidArgument, "use CancelOrderByID to cancel an order")
	}
	from, ok := allowedPredecessors[input.Order.Status]
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "status %q can not be set", input.Order.Status)
	}
	changed, err := s.OrderRepo.TransitionStatus(ctx, input.Order.ID, from, input.Order.Status)
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, status.Error(codes.FailedPrecondition, "order does not exist or its status can not change to the requested one")
	}
	return &dto.UpdateOrderByIDOutput{
		Message: "Update Order successfully",
		Success: true,
	}, nil
}

func (s *OrderService) CancelOrderByID(ctx context.Context, input *dto.CancelOrderByIDInput) (*dto.CancelOrderByIDOutput, error) {
	if err := s.OrderRepo.CancelOrderByID(ctx, input.ID); err != nil {
		switch {
		case errors.Is(err, repository.ErrOrderNotFound):
			return nil, status.Error(codes.NotFound, err.Error())
		case errors.Is(err, repository.ErrInvalidTransition):
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		return nil, err
	}
	return &dto.CancelOrderByIDOutput{
		Message: "Cancel Order successfully",
		Success: true,
	}, nil
}
