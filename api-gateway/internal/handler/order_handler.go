package handler

import (
	"api-gateway/internal/client/orderclient"

	"go.uber.org/zap"
)

// OrderHandler : handler for OrderClient
type OrderHandler struct {
	Service *orderclient.OrderClient
	Logger  *zap.Logger
}

// NewOrderHandler create new OrderHandler
func NewOrderHandler(service *orderclient.OrderClient, logger *zap.Logger) *OrderHandler {
	return &OrderHandler{
		Service: service,
		Logger:  logger,
	}
}
