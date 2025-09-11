package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
	"user-service/pkg/outbox"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

func (s *UserService) ProducerCreSelKafkaEventWorker(ctx context.Context, interval time.Duration, limit int, topic string) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			// Cancel by context
			case <-ctx.Done():
				s.ZapLogger.Info("UserService: Worker send CreateSeller Kafka event stop by context")
				return
			// Interval time
			case <-ticker.C:
				if err := s.producerCreateSellerKafkaEventBatch(ctx, limit, topic); err != nil {
					s.ZapLogger.Warn("UserService: error in procedure CreSelKafkaEvent batch", zap.Error(err))
				}
			}
		}
	}()
}

func (s *UserService) producerCreateSellerKafkaEventBatch(ctx context.Context, limit int, topic string) error {
	// Create context for function
	ctxEachEvent, cancel := context.WithTimeout(ctx, 9*time.Second)
	defer cancel()

	// Get models from DB
	eventsModel, err := s.UserRepo.GetCreateSellerEventNotPublish(limit)
	if err != nil {
		log.Println("Can not get KafkaEvent from OutboxDB")
		return err
	}

	var firstErr error
	for _, eventModel := range eventsModel {
		fmt.Println(eventModel)
		if err := s.producerCreateSellerKafkaEvent(ctxEachEvent, eventModel, topic); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *UserService) producerCreateSellerKafkaEvent(ctx context.Context, eventModel *outbox.CreateSellerKafkaEvent, topic string) error {
	// Parse event model to json
	eventJson, err := json.Marshal(eventModel)
	if err != nil {
		log.Printf("Can not marshal event: %v with err: %v\n", eventJson, err)
		return err
	}
	// Publish event
	if err := s.MQProducer.Publish(ctx, &kafka.LeastBytes{}, topic, []byte("key"), eventJson); err != nil {
		s.ZapLogger.Warn("UserService: publish to Kafka failure", zap.Error(err))
		if err2 := s.UserRepo.UpdateCreateSellerEventStatus(ctx, eventModel.SellerID, "FAILED"); err2 != nil {
			s.ZapLogger.Warn("UserService: publish to Kafka failure and can not update OutboxDB")
			return err2
		}
		s.ZapLogger.Info("UserService: publish to Kafka failure and update OutboxDB success")
		return err
	}
	// Update OutboxDB if procedure successfully
	if err := s.UserRepo.UpdateCreateSellerEventStatus(ctx, eventModel.SellerID, "SUCCESS"); err != nil {
		s.ZapLogger.Warn("UserService: publish to Kafka success but update to OutboxDB failed")
		return err
	}

	s.ZapLogger.Info("UserService: publish to Kafka success")
	return nil
}
