package handler

import (
	"api-gateway/internal/client/userclient"

	"go.uber.org/zap"
)

// UserHandler : handler for UserClient
type UserHandler struct {
	Service *userclient.UserClient
	Logger  *zap.Logger
}

// NewUserHandler create new UserHandler
func NewUserHandler(service *userclient.UserClient, logger *zap.Logger) *UserHandler {
	return &UserHandler{
		Service: service,
		Logger:  logger,
	}
}
