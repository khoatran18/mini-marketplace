package main

import (
	"ProjectPhase2/api-gateway/internal/config"
	"ProjectPhase2/api-gateway/internal/router"

	"github.com/gin-gonic/gin"
	"github.com/lpernett/godotenv"
)

func main() {
	godotenv.Load(".env")

	// Load config for redis, zap logger, ...
	serviceConfig := config.NewServiceConfig()

	// Setup router
	engine := gin.New()
	router.SetupRouter(engine, serviceConfig)

	// Run
	engine.Run(":8080")
}
