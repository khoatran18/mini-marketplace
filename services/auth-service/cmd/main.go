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
		log.Fatal("Error NewServiceConfig")
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
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	authpb.RegisterAuthServiceServer(s, &server.AuthServer{
		AuthService: authService,
		ZapLogger:   serviceConfig.ZapLogger,
	})

	// Test
	topic := "auth.change_password"
	conn, err := kafka.DialLeader(context.Background(), "tcp", "localhost:9092", topic, 0)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	// Create go routine for publishing PwdVersion Kafka to API Gateway
	ctx := context.Context(context.Background())
	authService.ProducerPwdVerKafkaEventWorker(ctx, 3*time.Second, 100, topic)

	// Run
	log.Printf("Auth Server listening at %v", lis.Addr())
	reflection.Register(s)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
