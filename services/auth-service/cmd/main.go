package main

import (
	"auth-service/internal/config"
	"auth-service/internal/repository"
	"auth-service/internal/server"
	"auth-service/internal/service"
	"auth-service/pkg/model"
	authpb "auth-service/pkg/pb"
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/lpernett/godotenv"
	"github.com/segmentio/kafka-go"
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
	s := grpc.NewServer()
	authpb.RegisterAuthServiceServer(s, &authServer)

	// Init sample data
	SeedAccounts(&authServer)

	// Test
	topic1 := "auth.change_password"
	conn, err := kafka.DialLeader(context.Background(), "tcp", "broker1:9092", topic1, 0)
	if err != nil {
		panic(err)
	}
	defer conn.Close()
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
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
