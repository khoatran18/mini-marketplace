package main

import (
	"account-service/internal/handler"
	"account-service/internal/repository"
	"account-service/internal/service"
	"account-service/pkg/model"
	"log"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	dsn := "host=localhost user=postgres password=zalando dbname=postgres port=5432"

	// Sử dụng log.Fatalf thay cho panic để in ra lỗi
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Ghi log để biết đã kết nối thành công
	log.Println("Connected to database successfully.")

	// AutoMigrate để tạo bảng User
	err = db.AutoMigrate(&model.User{})
	if err != nil {
		log.Fatalf("Failed to migrate user: %v", err)
	}

	// Ghi log sau khi migrate
	log.Println("Database migration completed.")

	userRepo := repository.NewUserRepository(db)
	authService := service.NewAuthService(userRepo)
	authHandler := handler.NewAuthHandler(authService)

	router := gin.Default()

	router.POST("/register", authHandler.Register)

	// Ghi log trước khi chạy server
	log.Println("Server is starting on port 8080...")

	if err := router.Run(":8080"); err != nil {
		// Sử dụng log.Fatalf để báo lỗi khi không thể chạy server
		log.Fatalf("Failed to run server: %v", err)
	}
}
