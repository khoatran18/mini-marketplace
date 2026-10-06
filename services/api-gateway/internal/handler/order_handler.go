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

// CreateOrder is responsible for parse create order gin.context request.
// The buyer is always the authenticated user; prices and totals sent by the
// client are ignored and recomputed by the order service.
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

	var req dto.CreateOrderInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("OrderHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	if req.Order == nil || len(req.Order.OrderItems) == 0 {
		badRequest(c, "Order must contain at least one item")
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "User ID not found in context"})
		return
	}

	// Never trust identifiers, status or money fields from the client
	req.Order.ID = 0
	req.Order.BuyerID = userID
	req.Order.Status = "PENDING"
	req.Order.TotalPrice = 0
	for _, item := range req.Order.OrderItems {
		if item == nil || item.Quantity <= 0 {
			badRequest(c, "Item quantity must be greater than zero")
			return
		}
		item.ID = 0
		item.OrderID = 0
		item.Price = 0
	}

	res, err := h.Service.CreateOrder(&req)
	if err != nil {
		respondError(c, h.Logger, "OrderHandler: CreateOrder warn", err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// getOwnedOrder loads an order and verifies it belongs to the caller.
// A foreign order is reported as not found so IDs cannot be probed.
func (h *OrderHandler) getOwnedOrder(c *gin.Context) (*dto.GetOrderByIDOutput, bool) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "User ID not found in context"})
		return nil, false
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.Logger.Warn("OrderHandler invalid request", zap.Error(err))
		badRequest(c, "Invalid order id")
		return nil, false
	}

	res, err := h.Service.GetOrderByID(&dto.GetOrderByIDInput{ID: id})
	if err != nil {
		respondError(c, h.Logger, "OrderHandler: GetOrderByID warn", err)
		return nil, false
	}
	if res.Order == nil || res.Order.BuyerID != userID {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Order not found"})
		return nil, false
	}
	return res, true
}

// GetOrderByID is responsible for parse get order by ID gin.context request
// GetOrderByID godoc
// @Summary GetOrderByID
// @Description Get one of the caller's orders
// @Tags order
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path integer true "Order ID"
// @Success 200 {object} dto.GetOrderByIDOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /orders/{id} [get]
func (h *OrderHandler) GetOrderByID(c *gin.Context) {
	res, ok := h.getOwnedOrder(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, res)
}

// GetOrdersByBuyerIDStatus lists the caller's orders by status. The buyer is taken
// from the token; a buyer_id query parameter is ignored.
// GetOrdersByBuyerIDStatus godoc
// @Summary GetOrdersByBuyerIDStatus
// @Description List the caller's orders by status
// @Tags order
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param status query string true "Status of order"
// @Success 200 {object} dto.GetOrdersByBuyerIDStatusOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /orders [get]
func (h *OrderHandler) GetOrdersByBuyerIDStatus(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "User ID not found in context"})
		return
	}

	res, err := h.Service.GetOrdersByBuyerIDStatus(&dto.GetOrdersByBuyerIDStatusInput{
		BuyerID: userID,
		Status:  c.Query("status"),
	})
	if err != nil {
		respondError(c, h.Logger, "OrderHandler: GetOrdersByBuyerIDStatus warn", err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// CancelOrderByID is responsible for parse cancel order by id gin.context request
// CancelOrderByID godoc
// @Summary CancelOrderByID
// @Description Cancel one of the caller's orders
// @Tags order
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path integer true "Order ID"
// @Success 200 {object} dto.CancelOrderByIDOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 422 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /orders/{id} [delete]
func (h *OrderHandler) CancelOrderByID(c *gin.Context) {
	owned, ok := h.getOwnedOrder(c)
	if !ok {
		return
	}

	res, err := h.Service.CancelOrderByID(&dto.CancelOrderByIDInput{ID: owned.Order.ID})
	if err != nil {
		respondError(c, h.Logger, "OrderHandler: CancelOrderByID warn", err)
		return
	}
	c.JSON(http.StatusOK, res)
}
