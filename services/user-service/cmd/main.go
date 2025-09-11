package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"
	"user-service/internal/client/clientmanager"
	"user-service/internal/client/serviceclientmanager"
	"user-service/internal/config"
	"user-service/internal/repository"
	"user-service/internal/server"
	"user-service/internal/service"
	userpb "user-service/pkg/pb"

	"github.com/lpernett/godotenv"
	"github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	godotenv.Load(".env")

	serviceConfig, err := config.NewServiceConfig()
	if err != nil {
		log.Fatal("Error NewServiceConfig: ", err.Error())
	}
	fmt.Println("Create ServiceConfig successfully!")

	_, err = config.NewEnvConfig()
	if err != nil {
		log.Fatal("Error NewEnvConfig", err.Error())
	}
	fmt.Println("Create EnvConfig successfully!")

	// err = serviceConfig.PostgresDB.AutoMigrate(&dto.Product{})
	if err != nil {
		log.Fatalf("Can not migrate database: %v", err)
	} else {
		fmt.Println("Migration successfully!")
	}

	grpcClientManager := clientmanager.NewClientManager()
	defer grpcClientManager.CloseAll()

	scm := serviceclientmanager.NewServiceClientManager(grpcClientManager, serviceConfig.ZapLogger)

	defer serviceConfig.KafkaInstance.KafkaManager.CloseWriterAll()
	defer serviceConfig.KafkaInstance.KafkaManager.CloseReaderAll()

	userRepo := repository.NewUserRepository(serviceConfig.PostgresDB)
	userService := service.NewUserService(userRepo, serviceConfig.ZapLogger, scm, serviceConfig.KafkaInstance.KafkaProducer, serviceConfig.KafkaInstance.KafkaConsumer, serviceConfig.KafkaInstance.KafkaClient)

	lis, err := net.Listen("tcp", ":50054")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// Test
	topic := "user.create_seller"
	conn, err := kafka.DialLeader(context.Background(), "tcp", "localhost:9092", topic, 0)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	// Create go routine for publishing PwdVersion Kafka to API Gateway
	ctx := context.Context(context.Background())
	userService.ProducerCreSelKafkaEventWorker(ctx, 3*time.Second, 100, topic)

	s := grpc.NewServer()
	userpb.RegisterUserServiceServer(s, &server.UserServer{
		UserService: userService,
		ZapLogger:   serviceConfig.ZapLogger,
	})
	log.Printf("Product Server Listen at %v", lis.Addr())

	reflection.Register(s)

	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}

}
