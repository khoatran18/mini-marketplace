package config

import (
	"context"
	"fmt"
	"os"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type ServiceConfig struct {
	ZapLogger   *zap.Logger
	RedisClient *redis.Client
}

func InitZapLogger() (*zap.Logger, error) {
	cfg := zap.Config{
		Level:             zap.NewAtomicLevelAt(zap.InfoLevel),
		Development:       true,
		DisableCaller:     false,
		DisableStacktrace: true,
		Encoding:          "json",
		EncoderConfig:     zap.NewProductionEncoderConfig(),
		OutputPaths:       []string{"stderr"},
		ErrorOutputPaths:  []string{"stderr"},
	}
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncoderConfig.TimeKey = "timestamp"

	logger, err := cfg.Build()
	if err != nil {
		return nil, err
	}
	fmt.Println("Init zap logger successfully!")

	return logger, nil
}

func InitRedisClient() (*redis.Client, error) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
		fmt.Println("REDIS_ADDR env variable not set, using default localhost:6379")
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	_, err := rdb.Ping(context.Background()).Result()
	if err != nil {
		return nil, err
	}

	fmt.Println("Init redis client successfully!")
	return rdb, nil
}

func NewServiceConfig() *ServiceConfig {
	zapLogger, err := InitZapLogger()
	if err != nil {
		panic(fmt.Sprintf("Can not init zap logger: %v", err))
	}

	redisClient, err := InitRedisClient()
	if err != nil {
		panic(fmt.Sprintf("Can not init redis client: %v", err))
	}

	return &ServiceConfig{
		ZapLogger:   zapLogger,
		RedisClient: redisClient,
	}
}
