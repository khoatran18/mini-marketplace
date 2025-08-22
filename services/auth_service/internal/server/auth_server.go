package server

import (
	"auth-service/internal/service"
	"auth-service/pkg/model"
	"auth-service/pkg/pb"
	"context"

	"buf.build/go/protovalidate"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AuthServer struct {
	pb.UnimplementedAuthServiceServer
	AuthService *service.AuthService
}

func (s *AuthServer) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	loginRequest := model.LoginRequest{
		Username: req.GetUsername(),
		Password: req.GetPassword(),
		Role:     req.GetRole(),
	}

	signedToken, err := s.AuthService.Login(&loginRequest)
	if err != nil {
		return &pb.LoginResponse{
			Message:     "Login failed!",
			AccessToken: "",
		}, err
	}

	return &pb.LoginResponse{
		Message:     "Login successfully!",
		AccessToken: signedToken,
	}, nil
}

func (s *AuthServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {

	if err := protovalidate.Validate(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	}

	registerRequest := model.RegisterRequest{
		Username: req.GetUsername(),
		Password: req.GetPassword(),
		Role:     req.GetRole(),
	}

	err := s.AuthService.Register(&registerRequest)
	if err != nil {
		return &pb.RegisterResponse{
			Message: "Register failed!",
		}, err
	}

	return &pb.RegisterResponse{
		Message: "Register successfully!",
	}, err
}
