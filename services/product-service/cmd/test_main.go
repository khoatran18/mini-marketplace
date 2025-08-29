package main

import (
	"fmt"
	"log"
	"net"
	"product-service/internal/config"
	"product-service/internal/repository"
	"product-service/internal/server"
	"product-service/internal/service"
	"product-service/pkg/model"
	"product-service/pkg/pb"

	"github.com/lpernett/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	godotenv.Load(".env")

	serviceConfig, err := config.NewServiceConfig()
	if err != nil {
		log.Fatal("Error NewServiceConfig: ", err.Error())
	}

	_, err = config.NewEnvConfig()
	if err != nil {
		log.Fatal("Error NewEnvConfig: ", err.Error())
	}

	// ✅ test kết nối trước khi migrate
	sqlDB, _ := serviceConfig.PostgresDB.DB()
	if pingErr := sqlDB.Ping(); pingErr != nil {
		log.Fatalf("[DB ERROR] Cannot connect to Postgres: %v", pingErr)
	} else {
		log.Println("[DB] Connected to Postgres successfully")
	}

	// ✅ migrate model
	err = serviceConfig.PostgresDB.AutoMigrate(&model.Product{})
	if err != nil {
		log.Fatalf("[DB ERROR] Can not migrate database: %v", err)
	} else {
		fmt.Println("Migration successfully!")
	}

	productRepo := repository.NewProductRepository(serviceConfig.PostgresDB)
	productService := service.NewProductService(productRepo, serviceConfig.ZapLogger)

	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterProductServiceServer(s, &server.ProductServer{
		ProductService: productService,
		ZapLogger:      serviceConfig.ZapLogger,
	})
	log.Printf("Product Server Listen at %v", lis.Addr())

	reflection.Register(s)

	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
