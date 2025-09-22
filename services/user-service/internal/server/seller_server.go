package server

import (
	"context"
	"user-service/internal/server/adapter"
	userpb "user-service/pkg/pb"

	"buf.build/go/protovalidate"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
)

func (s *UserServer) CreateSeller(ctx context.Context, req *userpb.CreateSellerRequest) (*userpb.CreateSellerResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("UserServer: invalid request for CreateSeller", zap.Error(err))
		return CreSelFailResponse("Invalid request for CreateSeller", err, codes.InvalidArgument)
	}
	input, err := adapter.CreSelRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("UserServer: parse CreateSeller request to input error", zap.Error(err))
		return CreSelFailResponse("Parse CreateSeller request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.UserService.CreateSeller(ctx, input)
	if err != nil {
		s.ZapLogger.Warn("UserServer: CreateSeller error in UserService", zap.Error(err))
		return CreSelFailResponse("CreateSeller error in UserService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := adapter.CreSelOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Error("UserServer: parse CreateSeller output to response error", zap.Error(err))
		return CreSelFailResponse("Parse CreateSeller output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("UserServer: invalid response for CreateSeller", zap.Error(err))
		return CreSelFailResponse("Invalid response for CreateSeller", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

func (s *UserServer) UpdateSellerByID(ctx context.Context, req *userpb.UpdateSellerByIDRequest) (*userpb.UpdateSellerByIDResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("UserServer: invalid request for UpdateSellerByID", zap.Error(err))
		return UpdSelByUseIDFailResponse("Invalid request for UpdateSellerByID", err, codes.InvalidArgument)
	}
	input, err := adapter.UpdSelByIDRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("UserServer: parse UpdateSellerByID request to input error", zap.Error(err))
		return UpdSelByUseIDFailResponse("Parse UpdateSellerByID request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.UserService.UpdateSellerByID(ctx, input)
	if err != nil {
		s.ZapLogger.Error("UserServer: UpdateSellerByID Error in UserService", zap.Error(err))
		return UpdSelByUseIDFailResponse("UpdateSellerByID Error in UserService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := adapter.UpdSelByIDOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Error("UserServer: parse UpdateSellerByID output to response error", zap.Error(err))
		return UpdSelByUseIDFailResponse("Parse UpdateSellerByID output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("UserServer: invalid response for UpdateSellerByID", zap.Error(err))
		return UpdSelByUseIDFailResponse("Invalid response for UpdateSellerByID", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

func (s *UserServer) GetSellerByID(ctx context.Context, req *userpb.GetSellerByIDRequest) (*userpb.GetSellerByIDResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("UserServer: invalid request for GetSellerByID", zap.Error(err))
		return GetSelByUseIDFailResponse("Invalid request for GetSellerByID", err, codes.InvalidArgument)
	}
	input, err := adapter.GetSelByIDRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("UserServer: parse GetSelByUseID request to input error", zap.Error(err))
		return GetSelByUseIDFailResponse("Parse GetSelByUseID request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.UserService.GetSellerByID(ctx, input)
	if err != nil {
		s.ZapLogger.Error("UserServer: GetSellerByID error in UserService", zap.Error(err))
		return GetSelByUseIDFailResponse("GetSellerByID error in UserService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := adapter.GetSelByIDOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Error("UserServer: parse GetSelByUseID output to response error", zap.Error(err))
		return GetSelByUseIDFailResponse("Parse GetSelByUseID output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("UserServer: invalid response for GetSelByUserID", zap.Error(err))
		return GetSelByUseIDFailResponse("Invalid response for GetSelByUserID", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}

func (s *UserServer) DelSellerByID(ctx context.Context, req *userpb.DelSellerByIDRequest) (*userpb.DelSellerByIDResponse, error) {

	// Validate ServerRequest and parse to ServiceInput
	if err := protovalidate.Validate(req); err != nil {
		s.ZapLogger.Warn("UserServer: invalid request for DelSelByUserID", zap.Error(err))
		return DelSelByIDFailResponse("Invalid request for DelSelByUserID", err, codes.InvalidArgument)
	}
	input, err := adapter.DelSelByIDRequestToInput(req)
	if err != nil {
		s.ZapLogger.Warn("UserServer: parse DelSellerByID request to input error", zap.Error(err))
		return DelSelByIDFailResponse("Parse DelSelByUserID request to input error", err, codes.InvalidArgument)
	}

	// Get ServiceOutput
	output, err := s.UserService.DelSellerByUserID(ctx, input)
	if err != nil {
		s.ZapLogger.Error("UserServer: DelSelByUserID error in UserService", zap.Error(err))
		return DelSelByIDFailResponse("DelSelByUserID error in UserService", err, codes.Internal)
	}

	// Parse ServiceOutput to ServerResponse and validate
	res, err := adapter.DelSelByIDOutputToResponse(output)
	if err != nil {
		s.ZapLogger.Error("UserServer: parse DelSelByUserID output to response error", zap.Error(err))
		return DelSelByIDFailResponse("Parse DelSelByUserID output to response error", err, codes.Unknown)
	}
	if err := protovalidate.Validate(res); err != nil {
		s.ZapLogger.Warn("UserServer: invalid response for DelSelByUserID", zap.Error(err))
		return DelSelByIDFailResponse("Invalid response for DelSelByUserID", err, codes.Internal)
	}

	// Return valid response
	return res, nil
}
