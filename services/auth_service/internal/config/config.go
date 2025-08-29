package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type ServiceConfig struct {
	ZapLogger   *zap.Logger
	RedisClient *redis.Client
	PostgresDB  *gorm.DB
}

type EnvConfig struct {
	JWTSecret     string
	JWTExpireTime time.Duration
}

// InitZapLogger init Zap Logger
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

// InitRedisClient init Redis Client
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

// InitPostgresDB init Postgres DB
func InitPostgresDB() (*gorm.DB, error) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		return nil, errors.New("POSTGRES_DSN env variable not set")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	db.AutoMigrate()

	return db, nil
}

// InitJWTSecret load env about jwt
func InitJWTSecret() (string, error) {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return "", errors.New("JWT secret not set")
	}
	return jwtSecret, nil
}

// InitJWTExpireTime load env about jwt expire time
func InitJWTExpireTime() (time.Duration, error) {
	jwtExpireTimeStr := os.Getenv("JWT_EXPIRE_TIME")
	jwtExpireTimeMinute, err := strconv.Atoi(jwtExpireTimeStr)
	if err != nil {
		fmt.Println("JWT_EXPIRE_TIME env variable not set, using default 5 minutes")
		jwtExpireTimeMinute = 5
	}

	jwtExpireTime := time.Duration(jwtExpireTimeMinute) * time.Minute

	return jwtExpireTime, nil
}

// NewServiceConfig init services: redis, database, zap logger, ...
func NewServiceConfig() (*ServiceConfig, error) {
	zapLogger, err := InitZapLogger()
	if err != nil {
		return nil, err
	}

	redisClient, err := InitRedisClient()
	if err != nil {
		return nil, err
	}

	postgresDB, err := InitPostgresDB()
	if err != nil {
		return nil, err
	}

	return &ServiceConfig{
		ZapLogger:   zapLogger,
		RedisClient: redisClient,
		PostgresDB:  postgresDB,
	}, nil
}

// NewEnvConfig load env config
func NewEnvConfig() (*EnvConfig, error) {
	jwtSecret, err := InitJWTSecret()
	if err != nil {
		return nil, err
	}

	jwtExpireTime, err := InitJWTExpireTime()
	if err != nil {
		return nil, err
	}

	return &EnvConfig{
		JWTSecret:     jwtSecret,
		JWTExpireTime: jwtExpireTime,
	}, nil
}
