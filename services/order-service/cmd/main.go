package main

import (
	"fmt"
	"log"
	"net"
	"order-service/internal/client/clientmanager"
	"order-service/internal/client/serviceclientmanager"
	"order-service/internal/config"
	"order-service/internal/repository"
	"order-service/internal/server"
	"order-service/internal/service"
	orderpb "order-service/pkg/pb"

	"github.com/lpernett/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	godotenv.Load(".env")

	serviceConfig, err := config.NewServiceConfig()
	if err != nil {
		log.Fatal("Error NewServiceConfig", err.Error())
	}

	_, err = config.NewEnvConfig()
	if err != nil {
		log.Fatal("Error NewEnvConfig", err.Error())
	}

	// err = serviceConfig.PostgresDB.AutoMigrate(&dto.Product{})
	if err != nil {
		log.Fatalf("Can not migrate database: %v", err)
	} else {
		fmt.Println("Migration successfully!")
	}

	grpcClientManager := clientmanager.NewClientManager()
	defer grpcClientManager.CloseAll()

	scm := serviceclientmanager.NewServiceClientManager(grpcClientManager, serviceConfig.ZapLogger)

	orderRepo := repository.NewOrderRepository(serviceConfig.PostgresDB)
	orderService := service.NewOrderService(orderRepo, serviceConfig.ZapLogger, scm)

	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	orderpb.RegisterOrderServiceServer(s, &server.OrderServer{
		OrderService: orderService,
		ZapLogger:    serviceConfig.ZapLogger,
	})

	log.Printf("Order Server Listen at %v", lis.Addr())

	reflection.Register(s)

	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
