package server

import (
	"user-service/internal/service"
	userpb "user-service/pkg/pb"

	"go.uber.org/zap"
)

type UserServer struct {
	userpb.UnimplementedUserServiceServer
	UserService *service.UserService
	ZapLogger   *zap.Logger
}
