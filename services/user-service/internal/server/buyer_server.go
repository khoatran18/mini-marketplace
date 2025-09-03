package server

import (
	"context"
	"user-service/internal/server/adapter"
	userpb "user-service/pkg/pb"

	"buf.build/go/protovalidate"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
)

func (s *UserServer) CreateBuyer(ctx context.Context, req *userpb.CreateBuyerRequest) (*userpb.CreateBuyerResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("UserServer: invalid request for CreateBuyer", zap.Error(err))
		return CreBuyFailResponse("Invalid request for CreateBuyer", err, codes.InvalidArgument)
	}
	input, err := adapter.CreBuyRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("UserServer: parse CreateBuyer request to input error", zap.Error(err))
		return CreBuyFailResponse("Parse CreateBuyer request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.UserService.CreateBuyer(ctx, input)
	if err != nil {
		s.ZapLogger.Warn("UserServer: CreateBuyer error in UserService", zap.Error(err))
		return CreBuyFailResponse("CreateBuyer error in UserService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := adapter.CreBuyOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Error("UserServer: parse CreateBuyer output to response error", zap.Error(err))
		return CreBuyFailResponse("Parse CreateBuyer output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("UserServer: invalid response for CreateBuyer", zap.Error(err))
		return CreBuyFailResponse("Invalid response for CreateBuyer", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

func (s *UserServer) UpdateBuyerByUserID(ctx context.Context, req *userpb.UpdateBuyerByUserIDRequest) (*userpb.UpdateBuyerByUserIDResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("UserServer: invalid request for UpdateBuyerByUserID", zap.Error(err))
		return UpdBuyByUseIDFailResponse("Invalid request for UpdateBuyerByUserID", err, codes.InvalidArgument)
	}
	input, err := adapter.UpdBuyByUseIDRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("UserServer: parse UpdateBuyerByUserID request to input error", zap.Error(err))
		return UpdBuyByUseIDFailResponse("Parse UpdateBuyerByUserID request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.UserService.UpdateBuyerByUserID(ctx, input)
	if err != nil {
		s.ZapLogger.Error("UserServer: UpdateBuyerByUserID Error in UserService", zap.Error(err))
		return UpdBuyByUseIDFailResponse("UpdateBuyerByUserID Error in UserService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := adapter.UpdBuyByUseIDOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Error("UserServer: parse UpdateBuyerByUserID output to response error", zap.Error(err))
		return UpdBuyByUseIDFailResponse("Parse UpdateBuyerByUserID output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("UserServer: invalid response for UpdateBuyerByUserID", zap.Error(err))
		return UpdBuyByUseIDFailResponse("Invalid response for UpdateBuyerByUserID", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

func (s *UserServer) GetBuyerByUserID(ctx context.Context, req *userpb.GetBuyerByUserIDRequest) (*userpb.GetBuyerByUserIDResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("UserServer: invalid request for GetBuyerByUserID", zap.Error(err))
		return GetBuyByUseIDFailResponse("Invalid request for GetBuyerByUserID", err, codes.InvalidArgument)
	}
	input, err := adapter.GetBuyByUseIDRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("UserServer: parse GetBuyByUseID request to input error", zap.Error(err))
		return GetBuyByUseIDFailResponse("Parse GetBuyByUseID request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.UserService.GetBuyerByUserID(ctx, input)
	if err != nil {
		s.ZapLogger.Error("UserServer: GetBuyerByUserID error in UserService", zap.Error(err))
		return GetBuyByUseIDFailResponse("GetBuyerByUserID error in UserService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := adapter.GetBuyByUseIDOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Error("UserServer: parse GetBuyByUseID output to response error", zap.Error(err))
		return GetBuyByUseIDFailResponse("Parse GetBuyByUseID output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("UserServer: invalid response for GetBuyByUserID", zap.Error(err))
		return GetBuyByUseIDFailResponse("Invalid response for GetBuyByUserID", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

func (s *UserServer) DelBuyerByUserID(ctx context.Context, req *userpb.DelBuyerByUserIDRequest) (*userpb.DelBuyerByUserIDResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("UserServer: invalid request for DelBuyerByUserID", zap.Error(err))
		return DelBuyByUseIDFailResponse("Invalid request for DelBuyerByUserID", err, codes.InvalidArgument)
	}
	input, err := adapter.DelBuyByUseIDRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("UserServer: parse DelBuyerByUserID request to input error", zap.Error(err))
		return DelBuyByUseIDFailResponse("Parse DelBuyerByUserID request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.UserService.DelBuyerByUserID(ctx, input)
	if err != nil {
		s.ZapLogger.Error("UserServer: DelBuyerByUserID error in UserService", zap.Error(err))
		return DelBuyByUseIDFailResponse("DelBuyerByUserID error in UserService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := adapter.DelBuyByUseIDOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Error("UserServer: parse DelBuyerByUserID output to response error", zap.Error(err))
		return DelBuyByUseIDFailResponse("Parse DelBuyerByUserID output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("UserServer: invalid response for DelBuyerByUserID", zap.Error(err))
		return DelBuyByUseIDFailResponse("Invalid response for DelBuyerByUserID", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}
