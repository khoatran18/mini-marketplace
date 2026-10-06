package main

import (
	"api-gateway/internal/client"
	"api-gateway/internal/config"
	"api-gateway/internal/handler"
	"api-gateway/internal/middleware"
	"api-gateway/internal/router"
	"api-gateway/internal/service"
	"api-gateway/pkg/clientname"
	"api-gateway/pkg/ops"
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lpernett/godotenv"
)

// @title           Swagger Example API
// @version         1.0
// @description     My API-Gateway server celler server.
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8080

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description JWT Authorization header using the Bearer scheme. Example: "Authorization: Bearer {token}"

// @externalDocs.description  OpenAPI
// @externalDocs.url          https://swagger.io/resources/open-api/

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

	managerHandler := handler.NewHandlerManager(grpcClientManager, envConfig.IPHashSecret, serviceConfig.ZapLogger)

	apiGatewayService := service.NewAPIGatewayService(serviceConfig.RedisClient, serviceConfig.KafkaInstance.KafkaProducer, serviceConfig.KafkaInstance.KafkaConsumer, serviceConfig.KafkaInstance.KafkaClient, serviceConfig.ZapLogger)

	// Keep cached password versions at least as long as an access token lives
	apiGatewayService.PwdVersionTTL = envConfig.JWTExpireTime + time.Minute

	// Run consumer in goroutine
	ctx := context.Context(context.Background())
	topic := "auth.change_password"
	for _, topic := range []string{topic} {
		if err := serviceConfig.KafkaInstance.KafkaClient.EnsureTopicExist(context.Background(), topic); err != nil {
			log.Fatalf("Can not ensure Kafka topic %s: %v", topic, err)
		}
	}
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
	engine.Use(gin.Recovery())

	// Admin HTTP port (/health /healthz /ready /readyz /metrics /version), internal only
	grpcAddrs := config.NewGRPCAddrConfig()
	checks := []ops.Check{
		ops.FuncCheck("redis", false, func(ctx context.Context) error { return serviceConfig.RedisClient.Ping(ctx).Err() }),
		ops.TCPCheck("kafka", os.Getenv("KAFKA_BROKERS_ADDR"), true),
	}
	for name, addr := range grpcAddrs {
		// payments and analytics are add-ons: the shop keeps working (COD, no dashboards) when they are down
		checks = append(checks, ops.GRPCHealthCheck(name, addr, name != clientname.PaymentClientName && name != clientname.AnalyticsClientName))
	}
	adm := ops.New(ops.Options{Service: "api-gateway", Checks: checks})
	engine.Use(middleware.MetricsMiddleware(adm.ObserveHTTP))
	// Only trust X-Forwarded-For from configured proxies (nil = trust none) so c.ClientIP() cannot be spoofed
	if err := engine.SetTrustedProxies(envConfig.TrustedProxies); err != nil {
		panic(err)
	}
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
	if err := adm.ServeHTTPMain(&http.Server{Addr: ":8080", Handler: engine, ReadHeaderTimeout: 10 * time.Second}); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
