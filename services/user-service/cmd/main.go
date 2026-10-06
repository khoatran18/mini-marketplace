package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"time"
	"user-service/internal/client/clientmanager"
	"user-service/internal/client/serviceclientmanager"
	"user-service/internal/config"
	"user-service/internal/repository"
	"user-service/internal/server"
	"user-service/internal/service"
	"user-service/pkg/model"
	"user-service/pkg/ops"
	userpb "user-service/pkg/pb"

	"github.com/lpernett/godotenv"
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

	if err := serviceConfig.PostgresDB.AutoMigrate(&model.Address{}); err != nil {
		log.Fatalf("Can not migrate addresses: %v", err)
	}

	userRepo := repository.NewUserRepository(serviceConfig.PostgresDB)
	userService := service.NewUserService(userRepo, serviceConfig.ZapLogger, scm, serviceConfig.KafkaInstance.KafkaProducer, serviceConfig.KafkaInstance.KafkaConsumer, serviceConfig.KafkaInstance.KafkaClient)

	lis, err := net.Listen("tcp", ":50054")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	topic := "user.create_seller"
	for _, topic := range []string{topic} {
		if err := serviceConfig.KafkaInstance.KafkaClient.EnsureTopicExist(context.Background(), topic); err != nil {
			log.Fatalf("Can not ensure Kafka topic %s: %v", topic, err)
		}
	}

	// Create go routine for publishing PwdVersion Kafka to API Gateway
	ctx := context.Context(context.Background())
	userService.ProducerCreSelKafkaEventWorker(ctx, 3*time.Second, 100, topic)

	// Admin HTTP port (/health /healthz /ready /readyz /metrics /version) next to the gRPC port
	adm := ops.New(ops.Options{
		Service: "user-service",
		Checks: []ops.Check{
			ops.SQLCheck("postgres", serviceConfig.PostgresDB.DB, true),
			ops.FuncCheck("redis", false, func(ctx context.Context) error { return serviceConfig.RedisClient.Ping(ctx).Err() }),
			ops.TCPCheck("kafka", os.Getenv("KAFKA_BROKERS_ADDR"), true),
			ops.GRPCHealthCheck("auth-service", config.NewGRPCAddrConfig()["AuthClientAddr"], true),
		},
	})
	if sqlDB, err := serviceConfig.PostgresDB.DB(); err == nil {
		adm.AttachDB(sqlDB)
	}
	s := grpc.NewServer(grpc.ChainUnaryInterceptor(adm.UnaryInterceptor()), grpc.ChainStreamInterceptor(adm.StreamInterceptor()))
	userpb.RegisterUserServiceServer(s, &server.UserServer{
		UserService: userService,
		ZapLogger:   serviceConfig.ZapLogger,
	})
	log.Printf("Product Server Listen at %v", lis.Addr())

	reflection.Register(s)

	if err := adm.ServeGRPC(s, lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}

}
