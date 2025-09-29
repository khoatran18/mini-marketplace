package handler

import (
	"api-gateway/internal/client/orderclient"
	"api-gateway/pkg/dto"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
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

func (h *OrderHandler) CreateOrder(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.CreateOrderInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("OrderHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
		return
	}

	// Get response and parse to json
	res, err := h.Service.CreateOrder(&req)
	if err != nil {
		h.Logger.Warn("OrderHandler: CreateOrder warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *OrderHandler) UpdateOrderByID(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.UpdateOrderByIDInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("OrderHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
		return
	}

	// Get ID
	idStr := c.Param("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.Logger.Warn("OrderHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
	}
	req.Order.ID = idUint

	// Get response and parse to json
	res, err := h.Service.UpdateOrderByID(&req)
	if err != nil {
		h.Logger.Warn("OrderHandler: UpdateOrderByID warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *OrderHandler) GetOrderByID(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.GetOrderByIDInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("OrderHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
		return
	}

	// Get ID
	idStr := c.Param("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.Logger.Warn("OrderHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
	}
	req.ID = idUint

	// Get response and parse to json
	res, err := h.Service.GetOrderByID(&req)
	if err != nil {
		h.Logger.Warn("OrderHandler: UpdateOrderByID warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *OrderHandler) GetOrdersByBuyerIDStatus(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.GetOrdersByBuyerIDStatusInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("OrderHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
		return
	}

	// Get ID
	buyerIDStr := c.Query("buyer_id")
	buyerIDUint, err := strconv.ParseUint(buyerIDStr, 10, 64)
	if err != nil {
		h.Logger.Warn("OrderHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
	}
	req.BuyerID = buyerIDUint
	req.Status = c.Query("status")

	// Get response and parse to json
	res, err := h.Service.GetOrdersByBuyerIDStatus(&req)
	if err != nil {
		h.Logger.Warn("OrderHandler: GetOrdersByBuyerIDStatus warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *OrderHandler) CancelOrderByID(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.CancelOrderByIDInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("OrderHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
		return
	}

	// Get ID
	idStr := c.Query("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.Logger.Warn("OrderHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
	}
	req.ID = idUint

	// Get response and parse to json
	res, err := h.Service.CancelOrderByID(&req)
	if err != nil {
		h.Logger.Warn("OrderHandler: CancelOrderByID warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}
