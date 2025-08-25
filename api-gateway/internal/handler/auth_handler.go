package handler

import (
	"api-gateway/internal/service"
	"api-gateway/pkg/model"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AuthHandler: handler for AuthService
type AuthHandler struct {
	Service *service.AuthService
	Logger  *zap.Logger
}

// NewAuthHandler create new AuthHandler
func NewAuthHandler(service *service.AuthService, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{
		Service: service,
		Logger:  logger,
	}
}

// Login is responsible for parse login gin.context request
func (authHandler *AuthHandler) Login(c *gin.Context) {

	// Parse from gin.context json to request model
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		authHandler.Logger.Warn("AuthHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get response and parse to json
	res, err := authHandler.Service.Login(&req)
	if err != nil {
		authHandler.Logger.Warn("AuthHandler: Login warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

// Register is responsible for parse login gin.context request
func (authHandler *AuthHandler) Register(c *gin.Context) {

	// Parse from gin.context json to request model
	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		authHandler.Logger.Warn("AuthHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get response and parse to json
	res, err := authHandler.Service.Register(&req)
	if err != nil {
		authHandler.Logger.Warn("AuthHandler Login warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

// ChangePassword is responsible for parse login gin.context request
func (authHandler *AuthHandler) ChangePassword(c *gin.Context) {

	// Parse from gin.context json to request model
	var req model.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		authHandler.Logger.Warn("AuthHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get response and parse to json
	res, err := authHandler.Service.ChangePassword(&req)
	if err != nil {
		authHandler.Logger.Warn("AuthHandler Login warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (authHandler *AuthHandler) RefreshToken(c *gin.Context) {

	// Parse from gin.context json to request model
	var req model.RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}

	// Get response and parse to json
	res, err := authHandler.Service.RefreshToken(&req)
	if err != nil {
		authHandler.Logger.Warn("AuthHandler Login warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}
