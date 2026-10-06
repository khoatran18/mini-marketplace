package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"payment-service/internal/config"
	"payment-service/internal/server"
	"payment-service/internal/service"
	"payment-service/pkg/events"
	"payment-service/pkg/ops"
	paymentpb "payment-service/pkg/pb"
	"sync/atomic"
	"time"

	"github.com/lpernett/godotenv"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

const workersInterval = time.Second

func main() {
	godotenv.Load(".env")

	// This service only implements the SIMULATED provider; refuse to start in any other mode
	if mode := os.Getenv("PAYMENTS_MODE"); mode != "" && mode != "mock" {
		log.Fatalf("PAYMENTS_MODE=%q is not supported: only the simulated provider (mock) exists", mode)
	}
	cfg, err := service.ConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	serviceConfig, err := config.NewServiceConfig()
	if err != nil {
		log.Fatal("Error NewServiceConfig: ", err)
	}
	if err := service.Migrate(serviceConfig.PostgresDB); err != nil {
		log.Fatalf("Can not migrate database: %v", err)
	}
	defer serviceConfig.KafkaInstance.KafkaManager.CloseWriterAll()
	defer serviceConfig.KafkaInstance.KafkaManager.CloseReaderAll()

	svc := service.NewPaymentService(serviceConfig.PostgresDB, serviceConfig.ZapLogger, cfg)

	ctx := context.Background()
	for _, topic := range []string{service.TopicPaymentRequested, service.TopicRefundRequested, service.TopicSucceeded, service.TopicFailed, service.TopicRefunded} {
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
	consume(service.TopicPaymentRequested, "payment-service-create", svc.HandlePaymentRequested)
	consume(service.TopicRefundRequested, "payment-service-refund", svc.HandleRefundRequested)

	events.Run(ctx, serviceConfig.PostgresDB, func(ctx context.Context, topic, key string, payload []byte) error {
		return serviceConfig.KafkaInstance.KafkaProducer.Publish(ctx, &kafka.Hash{}, topic, []byte(key), payload)
	}, 2*time.Second, 100, func(err error) { serviceConfig.ZapLogger.Warn("domain event publish failed", zap.Error(err)) })

	var lastTick atomic.Int64
	lastTick.Store(time.Now().Unix())
	svc.RunWorkers(ctx, workersInterval, func() { lastTick.Store(time.Now().Unix()) })

	lis, err := net.Listen("tcp", ":50057")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	adm := ops.New(ops.Options{
		Service: "payment-service",
		Checks: []ops.Check{
			ops.SQLCheck("postgres", serviceConfig.PostgresDB.DB, true),
			ops.FuncCheck("redis", false, func(ctx context.Context) error { return serviceConfig.RedisClient.Ping(ctx).Err() }),
			ops.TCPCheck("kafka", os.Getenv("KAFKA_BROKERS_ADDR"), true),
			ops.FuncCheck("workers", true, func(ctx context.Context) error {
				if time.Since(time.Unix(lastTick.Load(), 0)) > 30*workersInterval {
					return errors.New("stalled")
				}
				return nil
			}),
		},
		// the simulated provider notifies this endpoint (internal port only, HMAC signed)
		Mount: func(mux *http.ServeMux) { mux.HandleFunc("POST /internal/payments/webhook", svc.WebhookHandler()) },
	})
	if sqlDB, err := serviceConfig.PostgresDB.DB(); err == nil {
		adm.AttachDB(sqlDB)
	}
	adm.GaugeFunc("mm_outbox_pending", "Unpublished outbox rows.", map[string]string{"table": "domain_events"}, func() float64 {
		n, _, _ := events.Backlog(serviceConfig.PostgresDB)
		return float64(n)
	})
	adm.GaugeFunc("mm_outbox_oldest_age_seconds", "Age of the oldest unpublished outbox row.", map[string]string{"table": "domain_events"}, func() float64 {
		_, age, _ := events.Backlog(serviceConfig.PostgresDB)
		return age
	})
	adm.GaugeFunc("mm_worker_last_tick_timestamp_seconds", "Unix time of the last worker round.", map[string]string{"worker": "payment_workers"}, func() float64 { return float64(lastTick.Load()) })

	s := grpc.NewServer(grpc.ChainUnaryInterceptor(adm.UnaryInterceptor()), grpc.ChainStreamInterceptor(adm.StreamInterceptor()))
	paymentpb.RegisterPaymentServiceServer(s, &server.PaymentServer{Service: svc})
	reflection.Register(s)

	log.Printf("Payment Server listening at %v (simulated provider, dev=%v)", lis.Addr(), cfg.Dev)
	if err := adm.ServeGRPC(s, lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
