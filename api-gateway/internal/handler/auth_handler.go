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
func (authHandler *AuthHandler) Login(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.LoginInput
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

	// Parse from gin.context json to request dto
	var req dto.RegisterInput
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

	// Parse from gin.context json to request dto
	var req dto.ChangePasswordInput
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

	// Parse from gin.context json to request dto
	var req dto.RefreshTokenInput
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
