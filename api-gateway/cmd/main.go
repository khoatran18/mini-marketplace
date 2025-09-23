package main

import (
	"api-gateway/internal/client"
	"api-gateway/internal/config"
	"api-gateway/internal/handler"
	"api-gateway/internal/router"
	"api-gateway/internal/service"
	"context"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/lpernett/godotenv"
)

func main() {
	godotenv.Load(".env")

	// Load config for redis, zap logger, ...
	serviceConfig, err := config.NewServiceConfig()
	if err != nil {
		panic(err)
	}

	// Load env for jwt, ...
	envConfig, err := config.NewEnvConfig()
	if err != nil {
		panic(err)
	}

	grpcClientManager := client.NewClientManager()
	defer grpcClientManager.CloseAll()

	managerHandler := handler.NewHandlerManager(grpcClientManager, serviceConfig.ZapLogger)

	apiGatewayService := service.NewAPIGatewayService(serviceConfig.RedisClient, serviceConfig.KafkaInstance.KafkaProducer, serviceConfig.KafkaInstance.KafkaConsumer, serviceConfig.KafkaInstance.KafkaClient, serviceConfig.ZapLogger)

	// Run consumer in goroutine
	ctx := context.Context(context.Background())
	topic := "auth.change_password"
	go func() {
		if err := serviceConfig.KafkaInstance.KafkaConsumer.Consume(ctx, topic, "api-gateway-group-3", apiGatewayService.AddChaPwdVerToRedis); err != nil {
			log.Printf("Consumer stopped with error: %v", err)
		}
	}()

	// Test
	//topic1 := "test_topic"
	//conn, err := kafka.DialLeader(context.Background(), "tcp", "broker1:9092", topic1, 0)
	//if err != nil {
	//	panic(err)
	//}
	//defer conn.Close()

	// Setup router
	engine := gin.New()
	router.SetupRouter(engine, managerHandler, serviceConfig, envConfig)

	//// Tạo channel chờ signal
	//sigs := make(chan os.Signal, 1)
	//signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	//
	//// Chạy server trong goroutine
	//go func() {
	//	engine.Run(":8080")
	//}()
	//
	//fmt.Println("Gateway is running on :8080. Press Ctrl+C to exit...")
	//
	//// Chờ tín hiệu hủy
	//<-sigs
	//fmt.Println("Received stop signal, shutting down...")

	// Run
	engine.Run(":8080")
}
