package handler

import (
	"api-gateway/internal/client/productclient"

	"go.uber.org/zap"
)

// ProductHandler : handler for ProductClient
type ProductHandler struct {
	Service *productclient.ProductClient
	Logger  *zap.Logger
}

// NewProductHandler create new ProductHandler
func NewProductHandler(service *productclient.ProductClient, logger *zap.Logger) *ProductHandler {
	return &ProductHandler{
		Service: service,
		Logger:  logger,
	}
}
