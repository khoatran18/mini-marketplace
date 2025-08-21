package main

import (
	"auth-service/internal/config"
	"auth-service/internal/handler"
	"auth-service/internal/repository"
	"auth-service/internal/service"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/lpernett/godotenv"
)

func main() {
	godotenv.Load(".env")

	// Load config for redis, zap logger, ...
	serviceConfig, err := config.NewServiceConfig()
	if err != nil {
		panic(err)
	}

	// Load env for jwt, ...
	envConfig, err := config.NewEnvConfig()
	if err != nil {
		panic(err)
	}

	// Load repository, service, handler for Auth Service
	accountRepo := repository.NewAccountRepository(serviceConfig.PostgresDB)
	authService := service.NewAuthService(accountRepo, envConfig.JWTSecret, envConfig.JWTExpireTime)
	authHandler := handler.NewAuthHandler(authService)

	router := gin.Default()

	router.POST("/register", authHandler.RegisterRestAPI)
	router.POST("/login", authHandler.LoginRestAPI)

	// Ghi log trước khi chạy server
	log.Println("Server is starting on port 8080...")

	if err := router.Run(":8080"); err != nil {
		// Sử dụng log.Fatalf để báo lỗi khi không thể chạy server
		log.Fatalf("Failed to run server: %v", err)
	}
}
