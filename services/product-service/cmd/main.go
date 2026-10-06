package main

import (
	"context"
	"log"
	"net"
	"os"
	"product-service/internal/config"
	"product-service/internal/repository"
	"product-service/internal/server"
	"product-service/internal/service"
	"product-service/pkg/ops"
	productpb "product-service/pkg/pb"
	"time"

	"github.com/lpernett/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

const (
	topicCreateOrder   = "order.create_order"
	topicValidateOrder = "product.validate_order"
	topicCancelOrder   = "order.cancel_order"
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

	defer serviceConfig.KafkaInstance.KafkaManager.CloseWriterAll()
	defer serviceConfig.KafkaInstance.KafkaManager.CloseReaderAll()

	productRepo := repository.NewProductRepository(serviceConfig.PostgresDB)
	productService := service.NewProductService(productRepo, serviceConfig.ZapLogger, serviceConfig.KafkaInstance.KafkaProducer, serviceConfig.KafkaInstance.KafkaConsumer, serviceConfig.KafkaInstance.KafkaClient)

	// Topics must exist before readers/writers use them
	ctx := context.Background()
	for _, topic := range []string{topicCreateOrder, topicValidateOrder, topicCancelOrder} {
		if err := serviceConfig.KafkaInstance.KafkaClient.EnsureTopicExist(ctx, topic); err != nil {
			log.Fatalf("Can not ensure Kafka topic %s: %v", topic, err)
		}
	}

	// Reserve inventory for new orders
	go func() {
		if err := serviceConfig.KafkaInstance.KafkaConsumer.Consume(ctx, topicCreateOrder, "product-service-validate-order", productService.ValidateProductInventory); err != nil {
			log.Printf("Consumer stopped with error: %v", err)
		}
	}()
	// Release inventory of canceled orders
	go func() {
		if err := serviceConfig.KafkaInstance.KafkaConsumer.Consume(ctx, topicCancelOrder, "product-service-cancel-order", productService.ReleaseProductInventory); err != nil {
			log.Printf("Consumer stopped with error: %v", err)
		}
	}()

	// Publish validation results back to order-service
	productService.ProducerValOrdKafkaEventWorker(ctx, 3*time.Second, 100, topicValidateOrder)

	lis, err := net.Listen("tcp", ":50053")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	productServer := server.ProductServer{
		ProductService: productService,
		ZapLogger:      serviceConfig.ZapLogger,
	}
	// Admin HTTP port (/health /healthz /ready /readyz /metrics /version) next to the gRPC port
	adm := ops.New(ops.Options{
		Service: "product-service",
		Checks: []ops.Check{
			ops.SQLCheck("postgres", serviceConfig.PostgresDB.DB, true),
			ops.FuncCheck("redis", false, func(ctx context.Context) error { return serviceConfig.RedisClient.Ping(ctx).Err() }),
			ops.TCPCheck("kafka", os.Getenv("KAFKA_BROKERS_ADDR"), true),
		},
	})
	if sqlDB, err := serviceConfig.PostgresDB.DB(); err == nil {
		adm.AttachDB(sqlDB)
	}
	s := grpc.NewServer(grpc.ChainUnaryInterceptor(adm.UnaryInterceptor()), grpc.ChainStreamInterceptor(adm.StreamInterceptor()))
	productpb.RegisterProductServiceServer(s, &productServer)
	log.Printf("Product Server Listen at %v", lis.Addr())

	// Demo products only when explicitly requested, and only into an empty catalog
	if os.Getenv("SEED_DEMO_DATA") == "true" {
		SeedProducts(&productServer)
	}

	reflection.Register(s)

	if err := adm.ServeGRPC(s, lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
