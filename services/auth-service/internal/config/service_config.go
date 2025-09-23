package config

import (
	"auth-service/internal/config/messagequeue/kafkaimpl"
	"auth-service/pkg/model"
	"auth-service/pkg/outbox"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type ServiceConfig struct {
	ZapLogger     *zap.Logger
	RedisClient   *redis.Client
	PostgresDB    *gorm.DB
	KafkaInstance *KafkaInstance
}

type KafkaInstance struct {
	KafkaManager  *kafkaimpl.KafkaManager
	KafkaProducer *kafkaimpl.KafkaProducer
	KafkaConsumer *kafkaimpl.KafkaConsumer
	KafkaClient   *kafkaimpl.KafkaClient
}

// initZapLogger init Zap Logger
func initZapLogger() (*zap.Logger, error) {
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

// initRedisClient init Redis Client
func initRedisClient() (*redis.Client, error) {
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

	// Test
	ctx := context.Background()
	err = rdb.Set(ctx, "test_key", "hello world", 5*time.Second).Err()
	if err != nil {
		fmt.Println("Failed to SET:", err)
	} else {
		testRes, err := rdb.Get(ctx, "test_key").Result()
		if err != nil {
			fmt.Println("Failed to GET key:", err)
		}
		fmt.Printf("Test_res:%v\n", testRes)
		fmt.Println("SET test_key -> hello world")
	}

	return rdb, nil
}

// initPostgresDB init Postgres DB
func initPostgresDB() (*gorm.DB, error) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		return nil, errors.New("POSTGRES_DSN env variable not set")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	db.AutoMigrate(&model.Account{}, &outbox.PwdVersionEvent{})

	fmt.Println("Init postgres db successfully!")
	return db, nil
}

func initAllKafkaInstance() (*kafkaimpl.KafkaManager, *kafkaimpl.KafkaProducer, *kafkaimpl.KafkaConsumer, *kafkaimpl.KafkaClient, error) {
	brokers := os.Getenv("KAFKA_BROKERS_ADDR")
	if brokers == "" {
		return nil, nil, nil, nil, errors.New("KAFKA_BROKERS_ADDR env variable not set")
	}
	fmt.Printf("KAFKA_BROKERS_ADDR: %s\n", brokers)
	brokersList := strings.Split(brokers, ",")

	producerRetry := GetEnvIntWithDefault("KAFKA_PRODUCER_RETRY", 3)
	producerBackoff := GetEnvIntWithDefault("KAFKA_PRODUCER_BACKOFF", 100)
	consumerBackoff := GetEnvIntWithDefault("KAFKA_PRODUCER_BACKOFF", 100)

	kafkaManager := kafkaimpl.NewKafkaManager(brokersList)
	kafkaProducer := kafkaimpl.NewKafkaProducer(kafkaManager, producerRetry, time.Duration(producerBackoff)*time.Millisecond)
	kafkaConsumer := kafkaimpl.NewKafkaConsumer(kafkaManager, time.Duration(consumerBackoff)*time.Millisecond)
	kafkaClient := kafkaimpl.NewKafkaClient(brokersList)

	fmt.Println("Init all kafka instance successfully!")
	return kafkaManager, kafkaProducer, kafkaConsumer, kafkaClient, nil
}

// NewServiceConfig init services: redis, database, zap logger, ...
func NewServiceConfig() (*ServiceConfig, error) {
	zapLogger, err := initZapLogger()
	if err != nil {
		return nil, err
	}

	redisClient, err := initRedisClient()
	if err != nil {
		return nil, err
	}

	postgresDB, err := initPostgresDB()
	if err != nil {
		return nil, err
	}

	kafkaManager, kafkaProducer, kafkaConsumer, kafkaClient, err := initAllKafkaInstance()
	if err != nil {
		return nil, err
	}

	return &ServiceConfig{
		ZapLogger:   zapLogger,
		RedisClient: redisClient,
		PostgresDB:  postgresDB,
		KafkaInstance: &KafkaInstance{
			KafkaManager:  kafkaManager,
			KafkaProducer: kafkaProducer,
			KafkaConsumer: kafkaConsumer,
			KafkaClient:   kafkaClient,
		},
	}, nil
}

func GetEnvIntWithDefault(key string, defaultValue int) int {
	str := os.Getenv(key)
	if str == "" {
		fmt.Printf("%s env variable not set, using default %v\n", key, defaultValue)
		return defaultValue
	}
	i, err := strconv.Atoi(str)
	if err != nil {
		fmt.Printf("%s env variable setted but not valid, using default %v\n", key, defaultValue)
		return defaultValue
	}
	return i
}
