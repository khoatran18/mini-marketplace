package main

import (
	"context"
	"errors"
	"log"
	"net"
	"order-service/internal/client/clientmanager"
	"order-service/internal/client/serviceclientmanager"
	"order-service/internal/config"
	"order-service/internal/repository"
	"order-service/internal/server"
	"order-service/internal/service"
	"order-service/pkg/events"
	"order-service/pkg/ops"
	orderpb "order-service/pkg/pb"
	"os"
	"sync/atomic"
	"time"

	"github.com/lpernett/godotenv"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

const (
	// legacy topics (kept: the stock reservation protocol with product-service did not change)
	topicCreateOrder   = "order.create_order"
	topicValidateOrder = "product.validate_order"
	topicCancelOrder   = "order.cancel_order"

	timersInterval = 30 * time.Second
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

	// Schema (orders, items, history, checkouts, carts, outbox) and legacy data
	if err := repository.Migrate(serviceConfig.PostgresDB); err != nil {
		log.Fatalf("Can not migrate database: %v", err)
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
	for _, topic := range []string{
		topicCreateOrder, topicValidateOrder, topicCancelOrder,
		repository.TopicStatusChanged, repository.TopicRefundRequested, repository.TopicPaymentRequest,
		repository.TopicPaymentSucceed, repository.TopicPaymentRefunded,
	} {
		if err := serviceConfig.KafkaInstance.KafkaClient.EnsureTopicExist(ctx, topic); err != nil {
			log.Fatalf("Can not ensure Kafka topic %s: %v", topic, err)
		}
	}

	consume := func(topic, group string, handler func(context.Context, *kafka.Message) error) {
		go func() {
			if err := serviceConfig.KafkaInstance.KafkaConsumer.Consume(ctx, topic, group, handler); err != nil {
				log.Printf("Consumer of %s stopped with error: %v", topic, err)
			}
		}()
	}
	consume(topicValidateOrder, "order-service-validate-result", orderService.UpdateOrderStatusByKafka)
	consume(repository.TopicPaymentSucceed, "order-service-payment-succeeded", orderService.HandlePaymentSucceeded)
	consume(repository.TopicPaymentRefunded, "order-service-payment-refunded", orderService.HandlePaymentRefunded)

	// Transactional outbox (create_order, cancel_order, status_changed, refund_requested, payment.requested)
	events.Run(ctx, serviceConfig.PostgresDB, func(ctx context.Context, topic, key string, payload []byte) error {
		return serviceConfig.KafkaInstance.KafkaProducer.Publish(ctx, &kafka.Hash{}, topic, []byte(key), payload)
	}, 2*time.Second, 100, func(err error) { serviceConfig.ZapLogger.Warn("domain event publish failed", zap.Error(err)) })

	// Rows written by older versions in the previous outbox tables are still drained
	orderService.ProducerCreOrdKafkaEventWorker(ctx, 3*time.Second, 100, topicCreateOrder)
	orderService.ProducerCancelOrdKafkaEventWorker(ctx, 3*time.Second, 100, topicCancelOrder)

	// Payment deadline (unpaid orders release their stock) and auto-delivery
	var lastTick atomic.Int64
	lastTick.Store(time.Now().Unix())
	orderService.RunTimers(ctx, timersInterval, func() { lastTick.Store(time.Now().Unix()) })

	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// Admin HTTP port (/health /healthz /ready /readyz /metrics /version) next to the gRPC port
	adm := ops.New(ops.Options{
		Service: "order-service",
		Checks: []ops.Check{
			ops.SQLCheck("postgres", serviceConfig.PostgresDB.DB, true),
			ops.FuncCheck("redis", false, func(ctx context.Context) error { return serviceConfig.RedisClient.Ping(ctx).Err() }),
			ops.TCPCheck("kafka", os.Getenv("KAFKA_BROKERS_ADDR"), true),
			ops.GRPCHealthCheck("product-service", config.NewGRPCAddrConfig()["ProductClientAddr"], true),
			ops.FuncCheck("timers_worker", true, func(ctx context.Context) error {
				if time.Since(time.Unix(lastTick.Load(), 0)) > 3*timersInterval {
					return errors.New("stalled")
				}
				return nil
			}),
		},
	})
	if sqlDB, err := serviceConfig.PostgresDB.DB(); err == nil {
		adm.AttachDB(sqlDB)
	}
	adm.GaugeFunc("mm_outbox_pending", "Unpublished outbox rows.", map[string]string{"table": "domain_events"}, func() float64 { n, _ := orderRepo.Backlog(); return float64(n) })
	adm.GaugeFunc("mm_outbox_oldest_age_seconds", "Age of the oldest unpublished outbox row.", map[string]string{"table": "domain_events"}, func() float64 { _, a := orderRepo.Backlog(); return a })
	adm.GaugeFunc("mm_worker_last_tick_timestamp_seconds", "Unix time of the last timers round.", map[string]string{"worker": "order_timers"}, func() float64 { return float64(lastTick.Load()) })

	s := grpc.NewServer(grpc.ChainUnaryInterceptor(adm.UnaryInterceptor()), grpc.ChainStreamInterceptor(adm.StreamInterceptor()))
	orderpb.RegisterOrderServiceServer(s, &server.OrderServer{
		OrderService: orderService,
		ZapLogger:    serviceConfig.ZapLogger,
	})

	log.Printf("Order Server Listen at %v", lis.Addr())

	reflection.Register(s)

	if err := adm.ServeGRPC(s, lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
