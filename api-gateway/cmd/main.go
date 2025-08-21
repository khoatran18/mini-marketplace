package main

import (
	"api-gateway/internal/config"
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

	// Setup router
	engine := gin.New()
	router.SetupRouter(engine, serviceConfig, envConfig)

	// Run
	engine.Run(":8080")
}
