package handler

import (
	"api-gateway/internal/client/userclient"
	"api-gateway/pkg/dto"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
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

func (h *UserHandler) CreateBuyer(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.CreateBuyerInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
		return
	}

	// Get response and parse to json
	res, err := h.Service.CreateBuyer(&req)
	if err != nil {
		h.Logger.Warn("UserHandler: CreateBuyer warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *UserHandler) GetBuyerByUserID(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.GetBuyByUseIDInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
		return
	}

	// Get ID
	idStr := c.Param("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
	}
	req.UserID = idUint

	// Get response and parse to json
	res, err := h.Service.GetBuyerByUserID(&req)
	if err != nil {
		h.Logger.Warn("UserHandler: GetBuyerByUserID warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *UserHandler) UpdateBuyerByUserID(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.UpdBuyByUseIDInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
		return
	}

	// Get ID
	idStr := c.Param("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
	}
	req.Buyer.UserID = idUint

	// Get response and parse to json
	res, err := h.Service.UpdateBuyerByUserID(&req)
	if err != nil {
		h.Logger.Warn("UserHandler: UpdateBuyerByUserID warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *UserHandler) DelBuyerByUserID(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.DelBuyByUseIDInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
		return
	}

	// Get ID
	idStr := c.Param("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
	}
	req.UserID = idUint

	// Get response and parse to json
	res, err := h.Service.DelBuyerByUserID(&req)
	if err != nil {
		h.Logger.Warn("UserHandler: DelBuyerByUserID warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *UserHandler) CreateSeller(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.CreateSellerInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
		return
	}

	// Get response and parse to json
	res, err := h.Service.CreateSeller(&req)
	if err != nil {
		h.Logger.Warn("UserHandler: CreateSeller warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *UserHandler) GetSellerByID(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.GetSelByIDInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
		return
	}

	// Get ID
	idStr := c.Param("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
	}
	req.UserID = idUint

	// Get response and parse to json
	res, err := h.Service.GetSellerByID(&req)
	if err != nil {
		h.Logger.Warn("UserHandler: GetSellerByID warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *UserHandler) UpdateSellerByID(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.UpdSelByIDInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
		return
	}

	// Get ID
	idStr := c.Param("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
	}
	req.UserID = idUint

	// Get response and parse to json
	res, err := h.Service.UpdateSellerByID(&req)
	if err != nil {
		h.Logger.Warn("UserHandler: UpdateSellerByID warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *UserHandler) DelSellerByID(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.DelSelByIDInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
		return
	}

	// Get ID
	idStr := c.Param("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
	}
	req.UserID = idUint

	// Get response and parse to json
	res, err := h.Service.DelSellerByID(&req)
	if err != nil {
		h.Logger.Warn("UserHandler: DelSellerByID warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}
