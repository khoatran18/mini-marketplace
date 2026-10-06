package handler

import (
	"api-gateway/internal/client/authclient"
	"api-gateway/internal/client/userclient"
	"api-gateway/pkg/dto"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// UserHandler : handler for UserClient
type UserHandler struct {
	Auth    *authclient.AuthClient // resolves the caller's store (seller) ID
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

// ownsStore reports whether the caller's store is the given seller ID.
func (h *UserHandler) ownsStore(c *gin.Context, sellerID uint64) bool {
	userID, ok := currentUserID(c)
	if !ok {
		return false
	}
	storeID, _, err := h.Auth.GetStoreID(userID)
	return err == nil && storeID != 0 && storeID == sellerID
}

// requireOwnStore aborts with 403 unless the caller's store is the given seller ID.
// It returns the caller's user ID.
func (h *UserHandler) requireOwnStore(c *gin.Context, sellerID uint64) (uint64, bool) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "User ID not found in context"})
		return 0, false
	}
	storeID, _, err := h.Auth.GetStoreID(userID)
	if err != nil {
		respondError(c, h.Logger, "UserHandler: resolve store failed", err)
		return 0, false
	}
	if storeID == 0 || storeID != sellerID {
		c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "You can only manage your own store"})
		return 0, false
	}
	return userID, true
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

	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "User ID not found in context"})
		return
	}
	if req.Buyer == nil {
		badRequest(c, "Buyer is required")
		return
	}
	req.Buyer.UserID = userID

	// Get response and parse to json
	res, err := h.Service.CreateBuyer(&req)
	if err != nil {
		respondError(c, h.Logger, "UserHandler: CreateBuyer warn", err)
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
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "User ID not found in context"})
		return
	}
	if idUint != userID {
		c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "You can only access your own profile"})
		return
	}
	req.UserID = idUint

	// Get response and parse to json
	res, err := h.Service.GetBuyerByUserID(&req)
	if err != nil {
		respondError(c, h.Logger, "UserHandler: GetBuyerByUserID warn", err)
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
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "User ID not found in context"})
		return
	}
	if idUint != userID {
		c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "You can only access your own profile"})
		return
	}
	if req.Buyer == nil {
		badRequest(c, "Buyer is required")
		return
	}
	req.Buyer.UserID = idUint

	// Get response and parse to json
	res, err := h.Service.UpdateBuyerByUserID(&req)
	if err != nil {
		respondError(c, h.Logger, "UserHandler: UpdateBuyerByUserID warn", err)
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
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "User ID not found in context"})
		return
	}
	if idUint != userID {
		c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "You can only access your own profile"})
		return
	}
	req.UserID = idUint

	// Get response and parse to json
	res, err := h.Service.DelBuyerByUserID(&req)
	if err != nil {
		respondError(c, h.Logger, "UserHandler: DelBuyerByUserID warn", err)
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

	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "User ID not found in context"})
		return
	}
	if req.Seller == nil {
		badRequest(c, "Seller is required")
		return
	}
	req.UserID = userID
	req.Seller.ID = 0

	// Get response and parse to json
	res, err := h.Service.CreateSeller(&req)
	if err != nil {
		respondError(c, h.Logger, "UserHandler: CreateSeller warn", err)
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
		respondError(c, h.Logger, "UserHandler: GetSellerByID warn", err)
		return
	}
	if res.Seller != nil && !h.ownsStore(c, idUint) {
		// Public view of a store: never expose banking, tax or contact details
		res.Seller = &dto.Seller{ID: res.Seller.ID, Name: res.Seller.Name, Description: res.Seller.Description}
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
	userID, ok := h.requireOwnStore(c, idUint)
	if !ok {
		return
	}
	if req.Seller == nil {
		badRequest(c, "Seller is required")
		return
	}
	req.UserID = userID // account that is checked for the seller_admin role
	req.Seller.ID = idUint

	// Get response and parse to json
	res, err := h.Service.UpdateSellerByID(&req)
	if err != nil {
		respondError(c, h.Logger, "UserHandler: UpdateSellerByID warn", err)
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
	if _, ok := h.requireOwnStore(c, idUint); !ok {
		return
	}
	req.UserID = idUint

	// Get response and parse to json
	res, err := h.Service.DelSellerByID(&req)
	if err != nil {
		respondError(c, h.Logger, "UserHandler: DelSellerByID warn", err)
		return
	}
	c.JSON(http.StatusOK, res)
}
