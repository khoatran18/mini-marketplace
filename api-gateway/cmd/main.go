package main

import (
	"api-gateway/internal/client"
	"api-gateway/internal/config"
	"api-gateway/internal/handler"
	"api-gateway/internal/router"
	"api-gateway/pkg/dto"
	"context"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/goccy/go-json"
	"github.com/lpernett/godotenv"
	"github.com/segmentio/kafka-go"
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

	// Chạy consumer trong goroutine
	ctx := context.Context(context.Background())
	topic := "auth.change_password"
	go func() {
		if err := serviceConfig.KafkaInstance.KafkaConsumer.Consume(ctx, topic, "api-gateway-group-3", func(ctx context.Context, msg *kafka.Message) error {
			var eventDTO dto.ChangePwdKafkaEvent
			if err := json.Unmarshal(msg.Value, &eventDTO); err != nil {
				return err
			}
			log.Printf("Received message: %s, PwdVersion: %v", string(msg.Value), eventDTO.PwdVersion)
			return nil
		}); err != nil {
			log.Printf("Consumer stopped with error: %v", err)
		}
	}()

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
