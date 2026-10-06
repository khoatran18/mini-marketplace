package handler

import (
	"api-gateway/internal/client/authclient"
	"api-gateway/pkg/dto"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AuthHandler : handler for AuthClient
type AuthHandler struct {
	Service *authclient.AuthClient
	Logger  *zap.Logger
}

// NewAuthHandler create new AuthHandler
func NewAuthHandler(service *authclient.AuthClient, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{
		Service: service,
		Logger:  logger,
	}
}

// Login is responsible for parse login gin.context request
// Login godoc
// @Summary Login
// @Description Get username, password and return token
// @Tags auth
// @Accept json
// @Produce json
// @Param input body dto.LoginInput true "Username and password to Login"
// @Success 200 {object} dto.LoginOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.LoginInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("AuthHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}

	// Get response and parse to json
	res, err := h.Service.Login(&req)
	if err != nil {
		h.Logger.Warn("AuthHandler: Login warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

// Register is responsible for parse register gin.context request
// Register godoc
// @Summary Register
// @Description Create account from username, password
// @Tags auth
// @Accept json
// @Produce json
// @Param input body dto.RegisterInput true "Username and password to Register"
// @Success 200 {object} dto.RegisterOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.RegisterInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("AuthHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}

	// Get response and parse to json
	res, err := h.Service.Register(&req)
	if err != nil {
		h.Logger.Warn("AuthHandler Register warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

// ChangePassword is responsible for parse change password gin.context request
// ChangePassword godoc
// @Summary ChangePassword
// @Description Change password
// @Tags auth
// @Accept json
// @Produce json
// @Param input body dto.ChangePasswordInput true "Username, old and new password to ChangePassword"
// @Success 200 {object} dto.ChangePasswordOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /auth/change-password [post]
func (h *AuthHandler) ChangePassword(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.ChangePasswordInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("AuthHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}

	// Get response and parse to json
	res, err := h.Service.ChangePassword(&req)
	if err != nil {
		h.Logger.Warn("AuthHandler Change Password warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

// RefreshToken is responsible for parse refresh token gin.context request
// RefreshToken godoc
// @Summary RefreshToken
// @Description Refresh token
// @Tags auth
// @Accept json
// @Produce json
// @Param input body dto.RefreshTokenInput true "Refresh Token to refresh"
// @Success 200 {object} dto.RefreshTokenOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /auth/refresh-token [post]
func (h *AuthHandler) RefreshToken(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.RefreshTokenInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("AuthHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}

	// Get response and parse to json
	res, err := h.Service.RefreshToken(&req)
	if err != nil {
		h.Logger.Warn("AuthHandler Refresh Token warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

// RegisterSellerRoles is responsible for parse create roles of seller gin.context request
// RegisterSellerRoles godoc
// @Summary RegisterSellerRoles
// @Description Create seller role account from username, password
// @Tags auth
// @Accept json
// @Produce json
// @Param input body dto.RegisterSellerRolesInput true "Username and password used to register seller role account"
// @Success 200 {object} dto.RegisterSellerRolesOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /auth/register-seller-roles [post]
func (h *AuthHandler) RegisterSellerRoles(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.RegisterSellerRolesInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("AuthHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	id, ok := c.Get("userID")
	if !ok {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString("not valid user_id")})
		return
	}
	adminID, ok := id.(uint64)
	if !ok {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString("not valid user_id")})
		return
	}
	req.SellerAdminID = adminID

	// Get response and parse to json
	res, err := h.Service.RegisterSellerRoles(&req)
	if err != nil {
		h.Logger.Warn("AuthHandler RegisterSellerRoles warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}
