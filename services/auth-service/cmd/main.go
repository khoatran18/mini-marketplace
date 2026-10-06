package main

import (
	"auth-service/internal/config"
	"auth-service/internal/repository"
	"auth-service/internal/server"
	"auth-service/internal/service"
	"auth-service/pkg/model"
	"auth-service/pkg/ops"
	authpb "auth-service/pkg/pb"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/lpernett/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {

	// Load all config
	godotenv.Load(".env")
	serviceConfig, err := config.NewServiceConfig()
	if err != nil {
		log.Fatal("Error NewServiceConfig", err)
	}
	envConfig, err := config.NewEnvConfig()
	if err != nil {
		log.Fatal("Error NewEnvConfig: ", err)
	}
	err = serviceConfig.PostgresDB.AutoMigrate(&model.Account{})
	if err != nil {
		log.Fatalf("Can not migrate database: %v", err)
	} else {
		fmt.Println("Migration successfully!")
	}

	defer serviceConfig.KafkaInstance.KafkaManager.CloseWriterAll()
	defer serviceConfig.KafkaInstance.KafkaManager.CloseReaderAll()

	// Create Repository, Service
	accountRepo := repository.NewAccountRepository(serviceConfig.PostgresDB)
	authService := service.NewAuthService(accountRepo, envConfig.JWTSecret, envConfig.JWTExpireTime, serviceConfig.ZapLogger,
		serviceConfig.KafkaInstance.KafkaProducer, serviceConfig.KafkaInstance.KafkaConsumer, serviceConfig.KafkaInstance.KafkaClient)

	// Create Server
	authServer := server.AuthServer{
		AuthService: authService,
		ZapLogger:   serviceConfig.ZapLogger,
	}
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	// Admin HTTP port (/health /healthz /ready /readyz /metrics /version) next to the gRPC port
	adm := ops.New(ops.Options{
		Service: "auth-service",
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
	authpb.RegisterAuthServiceServer(s, &authServer)

	// Platform administrator (never self-registered): created once from ADMIN_BOOTSTRAP_USERNAME/PASSWORD
	if u, pw := os.Getenv("ADMIN_BOOTSTRAP_USERNAME"), os.Getenv("ADMIN_BOOTSTRAP_PASSWORD"); u != "" || pw != "" {
		created, err := authService.EnsureAdmin(context.Background(), u, pw)
		if err != nil {
			log.Fatalf("ADMIN_BOOTSTRAP_* rejected: %v", err)
		}
		log.Printf("Admin bootstrap: user %q (created=%v)", u, created)
	}

	// Demo accounts only when explicitly requested (never in production)
	if os.Getenv("SEED_DEMO_DATA") == "true" {
		SeedAccounts(&authServer)
	}

	// Topics must exist before readers/writers use them
	topic1 := "auth.change_password"
	for _, topic := range []string{topic1, "user.create_seller"} {
		if err := serviceConfig.KafkaInstance.KafkaClient.EnsureTopicExist(context.Background(), topic); err != nil {
			log.Fatalf("Can not ensure Kafka topic %s: %v", topic, err)
		}
	}
	ctx1 := context.Context(context.Background())
	authService.ProducerPwdVerKafkaEventWorker(ctx1, 3*time.Second, 100, topic1)

	// Chạy consumer trong goroutine
	ctx2 := context.Context(context.Background())
	topic2 := "user.create_seller"
	go func() {
		if err := serviceConfig.KafkaInstance.KafkaConsumer.Consume(ctx2, topic2, "auth-service", authService.UpdateStoreIDFromKafka); err != nil {
			log.Printf("Consumer stopped with error: %v", err)
		}
	}()

	// Run
	log.Printf("Auth Server listening at %v", lis.Addr())
	reflection.Register(s)
	if err := adm.ServeGRPC(s, lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
