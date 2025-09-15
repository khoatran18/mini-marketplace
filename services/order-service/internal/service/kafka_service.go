package service

import (
	"context"
	"encoding/json"
	"log"
	"order-service/internal/service/adapter"
	"order-service/pkg/dto"
	"order-service/pkg/outbox"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// For Consumer

func (s *OrderService) UpdateOrderStatusByKafka(ctx context.Context, msg *kafka.Message) error {
	var eventDTO dto.ValidateOrderKafkaEvent
	if err := json.Unmarshal(msg.Value, &eventDTO); err != nil {
		return err
	}

	var status string
	if eventDTO.Success == false {
		status = "FAILED"
	} else {
		status = "SUCCESS"
	}
	if err := s.OrderRepo.UpdateOrderStatusByID(ctx, eventDTO.OrderID, status); err != nil {
		return err
	}
	return nil
}

// For Producer

func (s *OrderService) ProducerCreOrdKafkaEventWorker(ctx context.Context, interval time.Duration, limit int, topic string) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			// Cancel by context
			case <-ctx.Done():
				s.ZapLogger.Info("OrderService: Worker send CreateOrder Kafka event stop by context")
				return
			// Interval time
			case <-ticker.C:
				if err := s.producerCreOrdKafkaEventBatch(ctx, limit, topic); err != nil {
					s.ZapLogger.Warn("OrderService: error in procedure CreOrdKafkaEvent batch", zap.Error(err))
				}
			}
		}
	}()
}

func (s *OrderService) producerCreOrdKafkaEventBatch(ctx context.Context, limit int, topic string) error {
	// Create context for function
	ctxEachEvent, cancel := context.WithTimeout(ctx, 9*time.Second)
	defer cancel()

	// Get models from DB
	eventsModel, err := s.OrderRepo.GetCreateOrderEventNotPublish(limit)
	if err != nil {
		log.Println("Can not get KafkaEvent from OutboxDB")
		return err
	}

	var firstErr error
	for _, eventModel := range eventsModel {
		eventKafka, err := adapter.CreOrdEvesModelToKafkaEvent(eventModel)
		if err != nil {
			firstErr = err
			continue
		}
		if err := s.producerPwdVersionKafkaEvent(ctxEachEvent, eventKafka, topic); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *OrderService) producerPwdVersionKafkaEvent(ctx context.Context, eventModel *outbox.CreateOrderKafkaEvent, topic string) error {

	// Parse event model to json
	log.Printf("Event model before marshal %+v\n", eventModel)
	eventJson, err := json.Marshal(eventModel)
	log.Printf("Event model after marshal %+v\n", eventJson)
	if err != nil {
		log.Printf("Can not marshal event: %v with err: %v\n", eventJson, err)
		return err
	}

	// Publish event
	if err := s.MQProducer.Publish(ctx, &kafka.LeastBytes{}, topic, []byte("key"), eventJson); err != nil {
		s.ZapLogger.Warn("OrderService: publish to Kafka failure", zap.Error(err))
		if err2 := s.OrderRepo.UpdateCreateOrderEventStatus(ctx, eventModel.OrderID, "FAILED"); err2 != nil {
			s.ZapLogger.Warn("OrderService: publish to Kafka failure and can not update OutboxDB")
			return err2
		}
		s.ZapLogger.Info("OrderService: publish to Kafka failure and update OutboxDB success")
		return err
	}
	// Update OutboxDB if procedure successfully
	if err := s.OrderRepo.UpdateCreateOrderEventStatus(ctx, eventModel.OrderID, "SUCCESS"); err != nil {
		s.ZapLogger.Warn("OrderService: publish to Kafka success but update to OutboxDB failed")
		return err
	}

	s.ZapLogger.Info("OrderService: publish to Kafka success")
	return nil
}

//func (s *OrderService) UpdateStoreIDFromKafka(ctx context.Context, msg *kafka.Message) error {
//
//	fmt.Println("UpdateStoreIDFromKafka")
//	var eventDTO dto.CreateSellerKafkaEvent
//	fmt.Printf("Msg value : %v\n", string(msg.Value))
//	if err := json.Unmarshal(msg.Value, &eventDTO); err != nil {
//		s.ZapLogger.Warn("OrderService: update store id error", zap.Error(err))
//		return err
//	}
//	fmt.Printf("%+v\n", eventDTO)
//	if err := s.OrderRepo.UpdateStoreID(ctx, eventDTO.UserID, eventDTO.SellerID); err != nil {
//		s.ZapLogger.Warn("OrderService: update store id error", zap.Error(err))
//		return err
//	}
//	s.ZapLogger.Info("OrderService: update store id successfully")
//	return nil
//}
