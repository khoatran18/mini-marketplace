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

// CreateBuyer is responsible for parse create buyer gin.context request
// CreateBuyer godoc
// @Summary CreateBuyer
// @Description Create new buyer
// @Tags user
// @Accept json
// @Produce json
// @Param request body dto.CreateBuyerInput true "Buyer DTO to create"
// @Security BearerAuth
// @Success 200 {object} dto.CreateBuyerOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /users/buyers [post]
func (h *UserHandler) CreateBuyer(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.CreateBuyerInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}

	// Get response and parse to json
	res, err := h.Service.CreateBuyer(&req)
	if err != nil {
		h.Logger.Warn("UserHandler: CreateBuyer warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

// GetBuyerByUserID is responsible for parse get buyer by ID gin.context request
// GetBuyerByUserID godoc
// @Summary GetBuyerByUserID
// @Description Get buyer by user_id
// @Tags user
// @Accept json
// @Produce json
// @Param id path integer true "User ID"
// @Security BearerAuth
// @Success 200 {object} dto.GetBuyByUseIDOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /users/buyers/{id} [get]
func (h *UserHandler) GetBuyerByUserID(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.GetBuyByUseIDInput
	//if err := c.ShouldBindJSON(&req); err != nil {
	//	h.Logger.Warn("UserHandler invalid request", zap.Error(err))
	//	c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
	//	return
	//}

	// Get ID
	idStr := c.Param("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	req.UserID = idUint

	// Get response and parse to json
	res, err := h.Service.GetBuyerByUserID(&req)
	if err != nil {
		h.Logger.Warn("UserHandler: GetBuyerByUserID warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

// UpdateBuyerByUserID is responsible for parse update buyer by user_id gin.context request
// UpdateBuyerByUserID godoc
// @Summary UpdateBuyerByUserID
// @Description Update buyer by user_id
// @Tags user
// @Accept json
// @Produce json
// @Param id path integer true "Buyer ID to update"
// @Param request body dto.UpdBuyByUseIDInput true "Buyer DTO to update"
// @Security BearerAuth
// @Success 200 {object} dto.UpdBuyByUseIDOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /users/buyers/{id} [put]
func (h *UserHandler) UpdateBuyerByUserID(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.UpdBuyByUseIDInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}

	// Get ID
	idStr := c.Param("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	req.Buyer.UserID = idUint

	// Get response and parse to json
	res, err := h.Service.UpdateBuyerByUserID(&req)
	if err != nil {
		h.Logger.Warn("UserHandler: UpdateBuyerByUserID warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

// DelBuyerByUserID is responsible for parse delete buyer by user_id gin.context request
// DelBuyerByUserID godoc
// @Summary DelBuyerByUserID
// @Description delete buyer by user_id
// @Tags user
// @Accept json
// @Produce json
// @Param id path integer true "Buyer ID to delete"
// @Security BearerAuth
// @Success 200 {object} dto.DelBuyByUseIDOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /users/buyers/{id} [delete]
func (h *UserHandler) DelBuyerByUserID(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.DelBuyByUseIDInput
	//if err := c.ShouldBindJSON(&req); err != nil {
	//	h.Logger.Warn("UserHandler invalid request", zap.Error(err))
	//	c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
	//	return
	//}

	// Get ID
	idStr := c.Param("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	req.UserID = idUint

	// Get response and parse to json
	res, err := h.Service.DelBuyerByUserID(&req)
	if err != nil {
		h.Logger.Warn("UserHandler: DelBuyerByUserID warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

// CreateSeller is responsible for parse create seller gin.context request
// CreateSeller godoc
// @Summary CreateSeller
// @Description Create new seller
// @Tags user
// @Accept json
// @Produce json
// @Param request body dto.CreateSellerInput true "Seller DTO to create"
// @Security BearerAuth
// @Success 200 {object} dto.CreateSellerOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /users/sellers [post]
func (h *UserHandler) CreateSeller(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.CreateSellerInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}

	// Get response and parse to json
	res, err := h.Service.CreateSeller(&req)
	if err != nil {
		h.Logger.Warn("UserHandler: CreateSeller warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

// GetSellerByID is responsible for parse get seller by ID gin.context request
// GetSellerByID godoc
// @Summary GetSellerByID
// @Description Get seller by ID
// @Tags user
// @Accept json
// @Produce json
// @Param id path integer true "Seller ID"
// @Security BearerAuth
// @Success 200 {object} dto.GetSelByIDOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /users/sellers/{id} [get]
func (h *UserHandler) GetSellerByID(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.GetSelByIDInput
	//if err := c.ShouldBindJSON(&req); err != nil {
	//	h.Logger.Warn("UserHandler invalid request", zap.Error(err))
	//	c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
	//	return
	//}

	// Get ID
	idStr := c.Param("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	req.UserID = idUint

	// Get response and parse to json
	res, err := h.Service.GetSellerByID(&req)
	if err != nil {
		h.Logger.Warn("UserHandler: GetSellerByID warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

// UpdateSellerByID is responsible for parse update seller by ID gin.context request
// UpdateSellerByID godoc
// @Summary UpdateSellerByID
// @Description Update seller by ID
// @Tags user
// @Accept json
// @Produce json
// @Param id path integer true "Seller ID to update"
// @Param request body dto.UpdSelByIDInput true "Seller DTO to update"
// @Security BearerAuth
// @Success 200 {object} dto.UpdSelByIDOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /users/sellers/{id} [put]
func (h *UserHandler) UpdateSellerByID(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.UpdSelByIDInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}

	// Get ID
	idStr := c.Param("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	req.UserID = idUint

	// Get response and parse to json
	res, err := h.Service.UpdateSellerByID(&req)
	if err != nil {
		h.Logger.Warn("UserHandler: UpdateSellerByID warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

// DelSellerByID is responsible for parse delete seller by ID gin.context request
// DelSellerByID godoc
// @Summary DelSellerByID
// @Description delete buyer by seller_id
// @Tags user
// @Accept json
// @Produce json
// @Param id path integer true "Seller ID to delete"
// @Security BearerAuth
// @Success 200 {object} dto.DelSelByIDOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /users/sellers/{id} [delete]
func (h *UserHandler) DelSellerByID(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.DelSelByIDInput
	//if err := c.ShouldBindJSON(&req); err != nil {
	//	h.Logger.Warn("UserHandler invalid request", zap.Error(err))
	//	c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
	//	return
	//}

	// Get ID
	idStr := c.Param("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.Logger.Warn("UserHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	req.UserID = idUint

	// Get response and parse to json
	res, err := h.Service.DelSellerByID(&req)
	if err != nil {
		h.Logger.Warn("UserHandler: DelSellerByID warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}
