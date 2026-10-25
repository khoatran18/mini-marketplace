package service

import (
	"mini-marketplace/shared/messagequeue"
	"mini-marketplace/shared/messagequeue/kafkaimpl"
	"user-service/internal/client/serviceclientmanager"
	"user-service/internal/repository"

	"go.uber.org/zap"
)

type UserService struct {
	UserRepo    *repository.UserRepository
	MQProducer  messagequeue.Producer
	MQConsumer  messagequeue.Consumer
	KafkaClient *kafkaimpl.KafkaClient
	SCM         *serviceclientmanager.ServiceClientManager
	ZapLogger   *zap.Logger
}

func NewUserService(userRepo *repository.UserRepository, zapLogger *zap.Logger, scm *serviceclientmanager.ServiceClientManager,
	producer messagequeue.Producer, consumer messagequeue.Consumer, client *kafkaimpl.KafkaClient) *UserService {
	return &UserService{
		UserRepo:    userRepo,
		MQProducer:  producer,
		MQConsumer:  consumer,
		KafkaClient: client,
		SCM:         scm,
		ZapLogger:   zapLogger,
	}
}
