package service

import (
	"api-gateway/internal/config/messagequeue"
	"api-gateway/internal/config/messagequeue/kafkaimpl"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type APIGatewayService struct {
	RedisClient *redis.Client
	MQProducer  messagequeue.Producer
	MQConsumer  messagequeue.Consumer
	KafkaClient *kafkaimpl.KafkaClient
	ZapLogger   *zap.Logger
	// PwdVersionTTL must be >= the access token lifetime: a cached password version
	// only matters while tokens issued before the password change can still be valid.
	PwdVersionTTL time.Duration
}

func NewAPIGatewayService(redisClient *redis.Client, producer messagequeue.Producer, consumer messagequeue.Consumer, kafkaClient *kafkaimpl.KafkaClient, zapLogger *zap.Logger) *APIGatewayService {
	return &APIGatewayService{
		RedisClient:   redisClient,
		MQProducer:    producer,
		MQConsumer:    consumer,
		KafkaClient:   kafkaClient,
		ZapLogger:     zapLogger,
		PwdVersionTTL: 5 * time.Minute,
	}
}
