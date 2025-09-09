package service

import (
	"api-gateway/internal/config/messagequeue"
	"api-gateway/internal/config/messagequeue/kafkaimpl"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type APIGatewayService struct {
	RedisClient *redis.Client
	MQProducer  messagequeue.Producer
	MQConsumer  messagequeue.Consumer
	KafkaClient *kafkaimpl.KafkaClient
	ZapLogger   *zap.Logger
}

func NewAPIGatewayService(redisClient *redis.Client, producer messagequeue.Producer, consumer messagequeue.Consumer, kafkaClient *kafkaimpl.KafkaClient, zapLogger *zap.Logger) *APIGatewayService {
	return &APIGatewayService{
		RedisClient: redisClient,
		MQProducer:  producer,
		MQConsumer:  consumer,
		KafkaClient: kafkaClient,
		ZapLogger:   zapLogger,
	}
}
