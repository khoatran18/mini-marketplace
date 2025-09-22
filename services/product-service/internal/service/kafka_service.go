package service

import (
	"context"
	"encoding/json"
	"log"
	"product-service/pkg/dto"
	"product-service/pkg/outbox"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// For consumer

func (s *ProductService) ValidateProductInventory(ctx context.Context, msg *kafka.Message) error {
	var eventDTO dto.CreateOrderKafkaEvent
	if err := json.Unmarshal(msg.Value, &eventDTO); err != nil {
		s.ZapLogger.Error("failed to unmarshal event", zap.Error(err))
		return err
	}

	// Check if this message is processed
	processed, err := s.ProductRepo.GetProcessedValOrdEvent(ctx, eventDTO.OrderID)
	if err != nil {
		return err
	}
	if processed {
		return nil
	}

	// Start handle
	if err := s.ProductRepo.GetAndDecreaseInventoryByIDBatch(ctx, &eventDTO); err != nil {
		s.ZapLogger.Error("failed to decrease inventory in batch", zap.Error(err))
		if strings.Contains(err.Error(), "errorDB") {
			s.ProductRepo.CreateOrUpdateValOrdEvent(s.ProductRepo.DB.WithContext(ctx), eventDTO.OrderID, false, false)
			return err
		}
		// Insert Outbox fail event
		s.ProductRepo.CreateOrUpdateValOrdEvent(s.ProductRepo.DB.WithContext(ctx), eventDTO.OrderID, false, true)
		return err
	}
	return nil
}

// For Producer

func (s *ProductService) ProducerValOrdKafkaEventWorker(ctx context.Context, interval time.Duration, limit int, topic string) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			// Cancel by context
			case <-ctx.Done():
				s.ZapLogger.Info("ProductService: Worker send ValidOrder Kafka event stop by context")
				return
			// Interval time
			case <-ticker.C:
				if err := s.producerValOrdKafkaEventBatch(ctx, limit, topic); err != nil {
					s.ZapLogger.Warn("ProductService: error in procedure ValOrdKafkaEvent batch", zap.Error(err))
				}
			}
		}
	}()

}

func (s *ProductService) producerValOrdKafkaEventBatch(ctx context.Context, limit int, topic string) error {
	// Create context for function
	ctxEachEvent, cancel := context.WithTimeout(ctx, 9*time.Second)
	defer cancel()

	// Get models from DB
	eventsModel, err := s.ProductRepo.GetValOrdEventNotPublish(limit)
	if err != nil {
		log.Println("Can not get KafkaEvent from OutboxDB")
		return err
	}

	var firstErr error
	for _, eventModel := range eventsModel {
		if err := s.producerValOrdKafkaEvent(ctxEachEvent, eventModel, topic); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *ProductService) producerValOrdKafkaEvent(ctx context.Context, eventModel *outbox.ValidateOrderKafkaEvent, topic string) error {
	// Parse event model to json
	eventJson, err := json.Marshal(eventModel)
	if err != nil {
		log.Printf("Can not marshal event: %v with err: %v\n", eventJson, err)
		return err
	}
	// Publish event
	if err := s.MQProducer.Publish(ctx, &kafka.LeastBytes{}, topic, []byte("key"), eventJson); err != nil {
		s.ZapLogger.Warn("ProductService: publish to Kafka failure", zap.Error(err))
		if err2 := s.ProductRepo.UpdateValOrdEventStatus(ctx, eventModel.OrderID, "FAILED"); err2 != nil {
			s.ZapLogger.Warn("ProductService: publish to Kafka failure and can not update OutboxDB")
			return err2
		}
		s.ZapLogger.Info("ProductService: publish to Kafka failure and update OutboxDB success")
		return err
	}
	// Update OutboxDB if procedure successfully
	if err := s.ProductRepo.UpdateValOrdEventStatus(ctx, eventModel.OrderID, "SUCCESS"); err != nil {
		s.ZapLogger.Warn("ProductService: publish to Kafka success but update to OutboxDB failed")
		return err
	}

	s.ZapLogger.Info("ProductService: publish to Kafka success")
	return nil
}

//func (s *ProductService) UpdateStoreIDFromKafka(ctx context.Context, msg *kafka.Message) error {
//
//	fmt.Println("UpdateStoreIDFromKafka")
//	var eventDTO dto.CreateOrderKafkaEvent
//	fmt.Printf("Msg value : %v\n", string(msg.Value))
//	if err := json.Unmarshal(msg.Value, &eventDTO); err != nil {
//		s.ZapLogger.Warn("ProductService: update store id error", zap.Error(err))
//		return err
//	}
//	fmt.Printf("%+v\n", eventDTO)
//	if err := s.ProductRepo.UpdateStoreID(ctx, eventDTO.UserID, eventDTO.SellerID); err != nil {
//		s.ZapLogger.Warn("ProductService: update store id error", zap.Error(err))
//		return err
//	}
//	s.ZapLogger.Info("ProductService: update store id successfully")
//	return nil
//}
