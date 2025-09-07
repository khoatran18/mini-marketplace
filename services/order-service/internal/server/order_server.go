package server

import (
	"context"
	"order-service/internal/server/adapter"
	"order-service/internal/service"
	orderpb "order-service/pkg/pb"

	"buf.build/go/protovalidate"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
)

type OrderServer struct {
	orderpb.UnimplementedOrderServiceServer
	OrderService *service.OrderService
	ZapLogger    *zap.Logger
}

func (s *OrderServer) CreateOrder(ctx context.Context, req *orderpb.CreateOrderRequest) (*orderpb.CreateOrderResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("OrderServer: invalid request for CreateProduct", zap.Error(err))
		return CreOrdFailResponse("Invalid request for CreateProduct", err, codes.InvalidArgument)
	}
	input, err := adapter.CreOrdRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("OrderServer: parse CreateOrder request to input error", zap.Error(err))
		return CreOrdFailResponse("Parse CreateOrder request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.OrderService.CreateOrder(ctx, input)
	if err != nil {
		s.ZapLogger.Warn("OrderServer: CreateOrder error in OrderService", zap.Error(err))
		return CreOrdFailResponse("CreateOrder error in OrderService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := adapter.CreOrdOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Warn("OrderServer: parse CreateOrder output to response error", zap.Error(err))
		return CreOrdFailResponse("Parse CreateOrder output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("OrderServer: invalid response for CreateOrder", zap.Error(err))
		return CreOrdFailResponse("Invalid response for CreateOrder", err, codes.InvalidArgument)
	}

	// Return valid response
	return res, nil
}

func (s *OrderServer) GetOrderByID(ctx context.Context, req *orderpb.GetOrderByIDRequest) (*orderpb.GetOrderByIDResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("OrderServer: invalid request for GetOrderByID", zap.Error(err))
		return GetOrdByIDFailResponse("Invalid request for GetOrderByID", err, codes.InvalidArgument)
	}
	input, err := adapter.GetOrdByIDRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("OrderServer: parse GetOrderByID request to input error", zap.Error(err))
		return GetOrdByIDFailResponse("Parse GetOrderByID request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.OrderService.GetOrderByID(ctx, input)
	if err != nil {
		s.ZapLogger.Warn("OrderServer: GetOrderByID error in OrderService", zap.Error(err))
		return GetOrdByIDFailResponse("GetOrderByID error in OrderService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := adapter.GetOrdByIDOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Warn("OrderServer: parse GetOrderByID output to response error", zap.Error(err))
		return GetOrdByIDFailResponse("Parse GetOrderByID output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("OrderServer: invalid response for GetOrderByID", zap.Error(err))
		return GetOrdByIDFailResponse("Invalid response for GetOrderByID", err, codes.InvalidArgument)
	}

	// Return valid response
	return res, nil
}

func (s *OrderServer) GetOrdersByBuyerIDStatus(ctx context.Context, req *orderpb.GetOrdersByBuyerIDStatusRequest) (*orderpb.GetOrdersByBuyerIDStatusResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("OrderServer: invalid request for GetOrdersByBuyerIDStatus", zap.Error(err))
		return GetOrdsByBuyIDStaFailResponse("Invalid request for GetOrdersByBuyerIDStatus", err, codes.InvalidArgument)
	}
	input, err := adapter.GetOrdsByBuyIDStaRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("OrderServer: parse GetOrdersByBuyerIDStatus request to input error", zap.Error(err))
		return GetOrdsByBuyIDStaFailResponse("Parse GetOrdersByBuyerIDStatus request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.OrderService.GetOrdersByBuyerIDStatus(ctx, input)
	if err != nil {
		s.ZapLogger.Warn("OrderServer: GetOrdersByBuyerIDStatus error in OrderService", zap.Error(err))
		return GetOrdsByBuyIDStaFailResponse("GetOrdersByBuyerIDStatus error in OrderService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := adapter.GetOrdsByBuyIDStaOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Warn("OrderServer: parse GetOrdersByBuyerIDStatus output to response error", zap.Error(err))
		return GetOrdsByBuyIDStaFailResponse("Parse GetOrdersByBuyerIDStatus output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("OrderServer: invalid response for GetOrdersByBuyerIDStatus", zap.Error(err))
		return GetOrdsByBuyIDStaFailResponse("Invalid response for GetOrdersByBuyerIDStatus", err, codes.InvalidArgument)
	}

	// Return valid response
	return res, nil
}

func (s *OrderServer) GetOrderItemsByOrderID(ctx context.Context, req *orderpb.GetOrderItemsByOrderIDRequest) (*orderpb.GetOrderItemsByOrderIDResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("OrderServer: invalid request for GetOrderItemsByOrderID", zap.Error(err))
		return GetOrdItesByOrdIDFailResponse("Invalid request for GetOrderItemsByOrderID", err, codes.InvalidArgument)
	}
	input, err := adapter.GetOrdItesByOrdIDRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("OrderServer: parse GetOrderItemsByOrderID request to input error", zap.Error(err))
		return GetOrdItesByOrdIDFailResponse("Parse GetOrderItemsByOrderID request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.OrderService.GetOrderItemsByOrderID(ctx, input)
	if err != nil {
		s.ZapLogger.Warn("OrderServer: GetOrderItemsByOrderID error in OrderService", zap.Error(err))
		return GetOrdItesByOrdIDFailResponse("GetOrderItemsByOrderID error in OrderService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := adapter.GetOrdItesByOrdIDOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Warn("OrderServer: parse GetOrderItemsByOrderID output to response error", zap.Error(err))
		return GetOrdItesByOrdIDFailResponse("Parse GetOrderItemsByOrderID output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("OrderServer: invalid response for GetOrderItemsByOrderID", zap.Error(err))
		return GetOrdItesByOrdIDFailResponse("Invalid response for GetOrderItemsByOrderID", err, codes.InvalidArgument)
	}

	// Return valid response
	return res, nil
}

func (s *OrderServer) UpdateOrderByID(ctx context.Context, req *orderpb.UpdateOrderByIDRequest) (*orderpb.UpdateOrderByIDResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("OrderServer: invalid request for UpdateOrderByID", zap.Error(err))
		return UpdOrdByIDFailResponse("Invalid request for UpdateOrderByID", err, codes.InvalidArgument)
	}
	input, err := adapter.UpdOrdByIDRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("OrderServer: parse UpdateOrderByID request to input error", zap.Error(err))
		return UpdOrdByIDFailResponse("Parse UpdateOrderByID request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.OrderService.UpdateOrderByID(ctx, input)
	if err != nil {
		s.ZapLogger.Warn("OrderServer: UpdateOrderByID error in OrderService", zap.Error(err))
		return UpdOrdByIDFailResponse("UpdateOrderByID error in OrderService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := adapter.UpdOrdByIDOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Warn("OrderServer: parse UpdateOrderByID output to response error", zap.Error(err))
		return UpdOrdByIDFailResponse("Parse UpdateOrderByID output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("OrderServer: invalid response for UpdateOrderByID", zap.Error(err))
		return UpdOrdByIDFailResponse("Invalid response for UpdateOrderByID", err, codes.InvalidArgument)
	}

	// Return valid response
	return res, nil
}

func (s *OrderServer) CancelOrderByID(ctx context.Context, req *orderpb.CancelOrderByIDRequest) (*orderpb.CancelOrderByIDResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("OrderServer: invalid request for UpdateOrderByID", zap.Error(err))
		return CanOrdByIDFailResponse("Invalid request for UpdateOrderByID", err, codes.InvalidArgument)
	}
	input, err := adapter.CanOrdByIDRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("OrderServer: parse UpdateOrderByID request to input error", zap.Error(err))
		return CanOrdByIDFailResponse("Parse UpdateOrderByID request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.OrderService.CancelOrderByID(ctx, input)
	if err != nil {
		s.ZapLogger.Warn("OrderServer: UpdateOrderByID error in OrderService", zap.Error(err))
		return CanOrdByIDFailResponse("UpdateOrderByID error in OrderService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := adapter.CanOrdByIDOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Warn("OrderServer: parse UpdateOrderByID output to response error", zap.Error(err))
		return CanOrdByIDFailResponse("Parse UpdateOrderByID output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("OrderServer: invalid response for UpdateOrderByID", zap.Error(err))
		return CanOrdByIDFailResponse("Invalid response for UpdateOrderByID", err, codes.InvalidArgument)
	}

	// Return valid response
	return res, nil
}
