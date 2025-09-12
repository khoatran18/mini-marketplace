package main

import (
	"context"
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
	"time"

	"github.com/lpernett/godotenv"
	"github.com/segmentio/kafka-go"
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

	defer serviceConfig.KafkaInstance.KafkaManager.CloseWriterAll()
	defer serviceConfig.KafkaInstance.KafkaManager.CloseReaderAll()

	scm := serviceclientmanager.NewServiceClientManager(grpcClientManager, serviceConfig.ZapLogger)

	orderRepo := repository.NewOrderRepository(serviceConfig.PostgresDB)
	orderService := service.NewOrderService(orderRepo, serviceConfig.ZapLogger, scm, serviceConfig.KafkaInstance.KafkaProducer, serviceConfig.KafkaInstance.KafkaConsumer, serviceConfig.KafkaInstance.KafkaClient)

	// Test
	topic1 := "order.create_order"
	conn, err := kafka.DialLeader(context.Background(), "tcp", "localhost:9092", topic1, 0)
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	ctx1 := context.Context(context.Background())
	orderService.ProducerCreOrdKafkaEventWorker(ctx1, 3*time.Second, 100, topic1)

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
