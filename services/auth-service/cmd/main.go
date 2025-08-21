package main

import (
	"account-service/internal/handler"
	"account-service/internal/repository"
	"account-service/internal/service"
	"account-service/pkg/model"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lpernett/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	godotenv.Load(".env")
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable not set")
	}
	jwtExpireTimeStr := os.Getenv("JWT_EXPIRE_TIME")
	jwtExpireTimeMinutes, err := strconv.Atoi(jwtExpireTimeStr)
	if err != nil {
		fmt.Printf("JWT_EXPIRE_TIME environment variable not set, using default 5 minutes")
		jwtExpireTimeMinutes = 5
	}

	jwtExpireTime := time.Minute * time.Duration(jwtExpireTimeMinutes)

	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		log.Fatal("POSTGRES_DSN environment variable not set")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	log.Println("Connected to database successfully.")

	err = db.AutoMigrate(&model.User{})
	if err != nil {
		log.Fatalf("Failed to migrate user: %v", err)
	}

	log.Println("Database migration completed.")

	userRepo := repository.NewUserRepository(db)
	authService := service.NewAuthService(userRepo, jwtSecret, jwtExpireTime)
	authHandler := handler.NewAuthHandler(authService)

	router := gin.Default()

	router.POST("/register", authHandler.Register)
	router.POST("/login", authHandler.Login)

	// Ghi log trước khi chạy server
	log.Println("Server is starting on port 8080...")

	if err := router.Run(":8080"); err != nil {
		// Sử dụng log.Fatalf để báo lỗi khi không thể chạy server
		log.Fatalf("Failed to run server: %v", err)
	}
}
