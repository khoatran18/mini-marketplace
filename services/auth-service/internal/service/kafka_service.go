package service

import (
	"auth-service/pkg/dto"
	"auth-service/pkg/outbox"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

func (s *AuthService) ProducerPwdVerKafkaEventWorker(ctx context.Context, interval time.Duration, limit int, topic string) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			// Cancel by context
			case <-ctx.Done():
				s.ZapLogger.Info("AuthService: Worker send CreateSeller Kafka event stop by context")
				return
			// Interval time
			case <-ticker.C:
				if err := s.producerPwdVersionKafkaEventBatch(ctx, limit, topic); err != nil {
					s.ZapLogger.Warn("AuthService: error in procedure CreSelKafkaEvent batch", zap.Error(err))
				}
			}
		}
	}()
}

func (s *AuthService) producerPwdVersionKafkaEventBatch(ctx context.Context, limit int, topic string) error {
	// Create context for function
	ctxEachEvent, cancel := context.WithTimeout(ctx, 9*time.Second)
	defer cancel()

	// Get models from DB
	eventsModel, err := s.AccountRepo.GetPwdVersionEventNotPublish(limit)
	if err != nil {
		log.Println("Can not get KafkaEvent from OutboxDB")
		return err
	}

	var firstErr error
	for _, eventModel := range eventsModel {
		if err := s.producerPwdVersionKafkaEvent(ctxEachEvent, eventModel, topic); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *AuthService) producerPwdVersionKafkaEvent(ctx context.Context, eventModel *outbox.PwdVersionKafkaEvent, topic string) error {
	// Parse event model to json
	eventJson, err := json.Marshal(eventModel)
	if err != nil {
		log.Printf("Can not marshal event: %v with err: %v\n", eventJson, err)
		return err
	}
	// Publish event
	if err := s.MQProducer.Publish(ctx, &kafka.LeastBytes{}, topic, []byte("key"), eventJson); err != nil {
		s.ZapLogger.Warn("AuthService: publish to Kafka failure", zap.Error(err))
		if err2 := s.AccountRepo.UpdatePwdVersionEventStatus(ctx, eventModel.UserID, "FAILED"); err2 != nil {
			s.ZapLogger.Warn("AuthService: publish to Kafka failure and can not update OutboxDB")
			return err2
		}
		s.ZapLogger.Info("AuthService: publish to Kafka failure and update OutboxDB success")
		return err
	}
	// Update OutboxDB if procedure successfully
	if err := s.AccountRepo.UpdatePwdVersionEventStatus(ctx, eventModel.UserID, "SUCCESS"); err != nil {
		s.ZapLogger.Warn("AuthService: publish to Kafka success but update to OutboxDB failed")
		return err
	}

	s.ZapLogger.Info("AuthService: publish to Kafka success")
	return nil
}

func (s *AuthService) UpdateStoreIDFromKafka(ctx context.Context, msg *kafka.Message) error {

	fmt.Println("UpdateStoreIDFromKafka")
	var eventDTO dto.CreateSellerKafkaEvent
	fmt.Printf("Msg value : %v\n", string(msg.Value))
	if err := json.Unmarshal(msg.Value, &eventDTO); err != nil {
		s.ZapLogger.Warn("AuthService: update store id error", zap.Error(err))
		return err
	}
	fmt.Printf("%+v\n", eventDTO)
	if err := s.AccountRepo.UpdateStoreID(ctx, eventDTO.UserID, eventDTO.SellerID); err != nil {
		s.ZapLogger.Warn("AuthService: update store id error", zap.Error(err))
		return err
	}
	s.ZapLogger.Info("AuthService: update store id successfully")
	return nil
}
