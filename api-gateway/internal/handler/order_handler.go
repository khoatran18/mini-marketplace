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

// CreateOrder is responsible for parse create order gin.context request
// CreateOrder godoc
// @Summary CreateOrder
// @Description Create new order
// @Tags order
// @Accept json
// @Produce json
// @Param request body dto.CreateOrderInput true "Order creation payload"
// @Security BearerAuth
// @Success 200 {object} dto.CreateOrderOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /orders [post]
func (h *OrderHandler) CreateOrder(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.CreateOrderInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("OrderHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}

	userIDInterface, exists := c.Get("userID")
	if !exists {
		c.JSON(400, gin.H{"error": "User ID not found in context"})
		return
	}
	userID, ok := userIDInterface.(uint64)
	if !ok {
		// Xử lý trường hợp ép kiểu thất bại (lỗi lập trình)
		c.JSON(500, gin.H{"error": "User ID format is incorrect"})
		return
	}

	req.Order.BuyerID = userID
	req.Order.Status = "PENDING"

	// Get response and parse to json
	res, err := h.Service.CreateOrder(&req)
	if err != nil {
		h.Logger.Warn("OrderHandler: CreateOrder warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

// UpdateOrderByID is responsible for parse update order by ID gin.context request
// UpdateOrderByID godoc
// @Summary UpdateOrderByID
// @Description Update order
// @Tags order
// @Accept json
// @Produce json
// @Param request body dto.UpdateOrderByIDInput true "Order update payload"
// @Security BearerAuth
// @Param id path integer true "Order ID"
// @Success 200 {object} dto.UpdateOrderByIDOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /orders/{id} [put]
func (h *OrderHandler) UpdateOrderByID(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.UpdateOrderByIDInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("OrderHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}

	// Get ID
	idStr := c.Param("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.Logger.Warn("OrderHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	req.Order.ID = idUint

	// Get response and parse to json
	res, err := h.Service.UpdateOrderByID(&req)
	if err != nil {
		h.Logger.Warn("OrderHandler: UpdateOrderByID warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

// GetOrderByID is responsible for parse get order by ID gin.context request
// GetOrderByID godoc
// @Summary GetOrderByID
// @Description Get order
// @Tags order
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path integer true "Order ID"
// @Success 200 {object} dto.GetOrderByIDOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /orders/{id} [get]
func (h *OrderHandler) GetOrderByID(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.GetOrderByIDInput
	//if err := c.ShouldBindJSON(&req); err != nil {
	//	h.Logger.Warn("OrderHandler invalid request", zap.Error(err))
	//	c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
	//	return
	//}

	// Get ID
	idStr := c.Param("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.Logger.Warn("OrderHandler invalid request", zap.Error(err))
		h.Logger.Warn("OrderHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	req.ID = idUint

	// Get response and parse to json
	res, err := h.Service.GetOrderByID(&req)
	if err != nil {
		h.Logger.Warn("OrderHandler: GetOrderByID warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

// GetOrdersByBuyerIDStatus is responsible for parse get orders by buyer_id and status gin.context request
// GetOrdersByBuyerIDStatus godoc
// @Summary GetOrdersByBuyerIDStatus
// @Description Get order by buyer_id and status
// @Tags order
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param buyer_id query integer true "Buyer ID"
// @Param status query string true "Status of order"
// @Success 200 {object} dto.GetOrdersByBuyerIDStatusOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /orders [get]
func (h *OrderHandler) GetOrdersByBuyerIDStatus(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.GetOrdersByBuyerIDStatusInput
	//if err := c.ShouldBindJSON(&req); err != nil {
	//	h.Logger.Warn("OrderHandler invalid request", zap.Error(err))
	//	c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
	//	return
	//}

	// Get ID
	buyerIDStr := c.Query("buyer_id")
	buyerIDUint, err := strconv.ParseUint(buyerIDStr, 10, 64)
	if err != nil {
		h.Logger.Warn("OrderHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	req.BuyerID = buyerIDUint
	req.Status = c.Query("status")

	// Get response and parse to json
	res, err := h.Service.GetOrdersByBuyerIDStatus(&req)
	if err != nil {
		h.Logger.Warn("OrderHandler: GetOrdersByBuyerIDStatus warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

// CancelOrderByID is responsible for parse cancel order by id gin.context request
// CancelOrderByID godoc
// @Summary CancelOrderByID
// @Description Cancel order by ID
// @Tags order
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path integer true "Order ID"
// @Success 200 {object} dto.CancelOrderByIDOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /orders/{id} [delete]
func (h *OrderHandler) CancelOrderByID(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.CancelOrderByIDInput
	//if err := c.ShouldBindJSON(&req); err != nil {
	//	h.Logger.Warn("OrderHandler invalid request", zap.Error(err))
	//	c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
	//	return
	//}

	// Get ID
	idStr := c.Param("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.Logger.Warn("OrderHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	req.ID = idUint

	// Get response and parse to json
	res, err := h.Service.CancelOrderByID(&req)
	if err != nil {
		h.Logger.Warn("OrderHandler: CancelOrderByID warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}
