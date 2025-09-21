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
func (h *AuthHandler) Login(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.LoginInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("AuthHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get response and parse to json
	res, err := h.Service.Login(&req)
	if err != nil {
		h.Logger.Warn("AuthHandler: Login warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

// Register is responsible for parse login gin.context request
func (h *AuthHandler) Register(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.RegisterInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("AuthHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get response and parse to json
	res, err := h.Service.Register(&req)
	if err != nil {
		h.Logger.Warn("AuthHandler Login warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

// ChangePassword is responsible for parse login gin.context request
func (h *AuthHandler) ChangePassword(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.ChangePasswordInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("AuthHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get response and parse to json
	res, err := h.Service.ChangePassword(&req)
	if err != nil {
		h.Logger.Warn("AuthHandler Login warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *AuthHandler) RefreshToken(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.RefreshTokenInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}

	// Get response and parse to json
	res, err := h.Service.RefreshToken(&req)
	if err != nil {
		h.Logger.Warn("AuthHandler Login warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *AuthHandler) RegisterSellerRoles(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.RegisterSellerRolesInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
	id, ok := c.Get("userID")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not valid user_id"})
	}
	adminID, ok := id.(uint64)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not valid user_id"})
	}
	req.SellerAdminID = adminID

	// Get response and parse to json
	res, err := h.Service.RegisterSellerRoles(&req)
	if err != nil {
		h.Logger.Warn("AuthHandler RegisterSellerRoles warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}
