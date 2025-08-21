package handler

import (
	"api-gateway/internal/middleware"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var globalRole = "user"

func Login(c *gin.Context) {
	userID := "test user id"
	userRole := globalRole

	claims := middleware.UserClaims{
		UserId: userID,
		Role:   userRole,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	jwtSecret := os.Getenv("JWT_SECRET")

	tokenString, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "Welcome to login",
		"token":   tokenString,
	})
}

func ProtectedEndpoint(c *gin.Context) {
	userID, _ := c.Get("userID")
	c.JSON(http.StatusOK, gin.H{
		"message": "Welcome to the Protected API",
		"user":    userID,
	})
}
