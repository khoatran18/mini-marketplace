package main

import (
	"api-gateway/internal/config"
	"api-gateway/internal/grpc_client"
	"api-gateway/internal/handler"
	"api-gateway/internal/router"

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

	grpcClientManager := grpc_client.NewClientManager()
	defer grpcClientManager.CloseAll()

	managerHandler := handler.NewHandlerManager(grpcClientManager, serviceConfig.ZapLogger)

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
