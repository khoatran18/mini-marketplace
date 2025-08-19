package main

import (
	"ProjectPhase2/api-gateway/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/lpernett/godotenv"
	"net/http"
	"os"
	"time"
)

var globalRole = "user"

func login(c *gin.Context) {
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

func protectedEndpoint(c *gin.Context) {
	userID, _ := c.Get("userID")
	c.JSON(http.StatusOK, gin.H{
		"message": "Welcome to the Protected API",
		"user":    userID})
}

func main() {
	godotenv.Load(".env")

	router := gin.New()

	middleware.InitMiddleware()

	// Start
	router.Use(middleware.RequestLoggingMiddleware())
	router.Use(middleware.RateLimitingMiddleware(100, time.Minute))
	router.POST("/login", login)

	protectedAPI := router.Group("/protected")
	protectedAPI.Use(
		middleware.AuthMiddleware(),
		middleware.AuthorizationMiddleware("user"),
	)
	{
		protectedAPI.GET("/profile", protectedEndpoint)
	}

	router.Run(":8080")
}
