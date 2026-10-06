package main

import (
	"context"
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

	grpcClientManager := clientmanager.NewClientManager()
	defer grpcClientManager.CloseAll()

	defer serviceConfig.KafkaInstance.KafkaManager.CloseWriterAll()
	defer serviceConfig.KafkaInstance.KafkaManager.CloseReaderAll()

	scm := serviceclientmanager.NewServiceClientManager(grpcClientManager, serviceConfig.ZapLogger)

	orderRepo := repository.NewOrderRepository(serviceConfig.PostgresDB)
	orderService := service.NewOrderService(orderRepo, serviceConfig.ZapLogger, scm, serviceConfig.KafkaInstance.KafkaProducer, serviceConfig.KafkaInstance.KafkaConsumer, serviceConfig.KafkaInstance.KafkaClient)

	// Topics must exist before readers/writers use them (auto-creation is disabled in kafka-go writers)
	ctx := context.Background()
	for _, topic := range []string{topicCreateOrder, topicValidateOrder, topicCancelOrder} {
		if err := serviceConfig.KafkaInstance.KafkaClient.EnsureTopicExist(ctx, topic); err != nil {
			log.Fatalf("Can not ensure Kafka topic %s: %v", topic, err)
		}
	}

	// Consume inventory validation results
	go func() {
		if err := serviceConfig.KafkaInstance.KafkaConsumer.Consume(ctx, topicValidateOrder, "order-service-validate-result", orderService.UpdateOrderStatusByKafka); err != nil {
			log.Printf("Consumer stopped with error: %v", err)
		}
	}()

	// Outbox publishers
	orderService.ProducerCreOrdKafkaEventWorker(ctx, 3*time.Second, 100, topicCreateOrder)
	orderService.ProducerCancelOrdKafkaEventWorker(ctx, 3*time.Second, 100, topicCancelOrder)

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
