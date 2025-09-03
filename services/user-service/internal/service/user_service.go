package service

import (
	"user-service/internal/repository"

	"go.uber.org/zap"
)

type UserService struct {
	UserRepo  *repository.UserRepository
	ZapLogger *zap.Logger
}

func NewUserService(userRepo *repository.UserRepository, zapLogger *zap.Logger) *UserService {
	return &UserService{
		UserRepo:  userRepo,
		ZapLogger: zapLogger,
	}
}
