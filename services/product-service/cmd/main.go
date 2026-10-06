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
	"product-service/pkg/events"
	"product-service/pkg/ops"
	productpb "product-service/pkg/pb"
	"time"

	"github.com/lpernett/godotenv"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

const (
	topicCreateOrder   = "order.create_order"
	topicValidateOrder = "product.validate_order"
	topicCancelOrder   = "order.cancel_order"
	// order-service announces every status change; SHIPPED moves reserved stock to sold
	topicOrderStatusChanged = "order.status_changed"
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

	// Schema, search helpers and reference data
	if err := repository.Migrate(serviceConfig.PostgresDB); err != nil {
		log.Fatalf("Can not migrate database: %v", err)
	}
	if os.Getenv("SEED_DEFAULT_CATEGORIES") != "false" {
		if err := repository.SeedCategories(serviceConfig.PostgresDB); err != nil {
			log.Fatalf("Can not seed categories: %v", err)
		}
	}

	productRepo := repository.NewProductRepository(serviceConfig.PostgresDB)
	productService := service.NewProductService(productRepo, serviceConfig.ZapLogger, serviceConfig.KafkaInstance.KafkaProducer, serviceConfig.KafkaInstance.KafkaConsumer, serviceConfig.KafkaInstance.KafkaClient)

	// Topics must exist before readers/writers use them
	ctx := context.Background()
	for _, topic := range []string{topicCreateOrder, topicValidateOrder, topicCancelOrder, topicOrderStatusChanged, repository.TopicProductChanged, repository.TopicInventoryChanged} {
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

	// Goods leave the warehouse when order-service reports SHIPPED
	go func() {
		if err := serviceConfig.KafkaInstance.KafkaConsumer.Consume(ctx, topicOrderStatusChanged, "product-service-ship-order", productService.MarkShippedFromOrderEvent); err != nil {
			log.Printf("Consumer stopped with error: %v", err)
		}
	}()

	// Publish product.changed / inventory.changed from the transactional outbox
	events.Run(ctx, serviceConfig.PostgresDB, func(ctx context.Context, topic, key string, payload []byte) error {
		return serviceConfig.KafkaInstance.KafkaProducer.Publish(ctx, &kafka.Hash{}, topic, []byte(key), payload)
	}, 2*time.Second, 100, func(err error) { serviceConfig.ZapLogger.Warn("domain event publish failed", zap.Error(err)) })

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
	// Outbox backlog (also shown on the Grafana "Kafka & Outbox" panels)
	backlog := func() (float64, float64) {
		n, age, _ := events.Backlog(serviceConfig.PostgresDB)
		return float64(n), age
	}
	adm.GaugeFunc("mm_outbox_pending", "Unpublished outbox rows.", map[string]string{"table": "domain_events"}, func() float64 { n, _ := backlog(); return n })
	adm.GaugeFunc("mm_outbox_oldest_age_seconds", "Age of the oldest unpublished outbox row.", map[string]string{"table": "domain_events"}, func() float64 { _, a := backlog(); return a })
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
