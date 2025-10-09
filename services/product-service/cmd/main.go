package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"product-service/internal/config"
	"product-service/internal/repository"
	"product-service/internal/server"
	"product-service/internal/service"
	productpb "product-service/pkg/pb"
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

	defer serviceConfig.KafkaInstance.KafkaManager.CloseWriterAll()
	defer serviceConfig.KafkaInstance.KafkaManager.CloseReaderAll()

	productRepo := repository.NewProductRepository(serviceConfig.PostgresDB)
	productService := service.NewProductService(productRepo, serviceConfig.ZapLogger, serviceConfig.KafkaInstance.KafkaProducer, serviceConfig.KafkaInstance.KafkaConsumer, serviceConfig.KafkaInstance.KafkaClient)

	// Run consumer in goroutine
	ctx := context.Context(context.Background())
	topic := "order.create_order"
	go func() {
		if err := serviceConfig.KafkaInstance.KafkaConsumer.Consume(ctx, topic, "order-service-group", productService.ValidateProductInventory); err != nil {
			log.Printf("Consumer stopped with error: %v", err)
		}
	}()

	// Run producer in goroutine
	topic1 := "product.validate_order"
	conn, err := kafka.DialLeader(context.Background(), "tcp", "broker1:9092", topic1, 0)
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	ctx1 := context.Context(context.Background())
	productService.ProducerValOrdKafkaEventWorker(ctx1, 10*time.Second, 100, topic1)

	lis, err := net.Listen("tcp", ":50053")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	productServer := server.ProductServer{
		ProductService: productService,
		ZapLogger:      serviceConfig.ZapLogger,
	}
	s := grpc.NewServer()
	productpb.RegisterProductServiceServer(s, &productServer)
	log.Printf("Product Server Listen at %v", lis.Addr())

	// --- Seed sample products ---
	SeedProducts(&productServer)

	reflection.Register(s)

	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
