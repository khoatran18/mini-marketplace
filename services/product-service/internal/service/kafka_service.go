package service

import (
	"context"
	"encoding/json"
	"errors"
	"product-service/internal/repository"
	"product-service/pkg/dto"
	"product-service/pkg/outbox"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// For consumer

func toItemQuantities(items []*dto.ItemEvent) []repository.ItemQuantity {
	out := make([]repository.ItemQuantity, 0, len(items))
	for _, item := range items {
		out = append(out, repository.ItemQuantity{ProductID: item.ProductID, Quantity: item.Quantity})
	}
	return out
}

// ValidateProductInventory reserves inventory for a new order and records the result, which the
// outbox worker publishes back to order-service. It is idempotent per order ID.
func (s *ProductService) ValidateProductInventory(ctx context.Context, msg *kafka.Message) error {
	var eventDTO dto.CreateOrderKafkaEvent
	if err := json.Unmarshal(msg.Value, &eventDTO); err != nil {
		// A malformed message can never succeed; drop it instead of retrying.
		s.ZapLogger.Error("failed to unmarshal event", zap.Error(err))
		return nil
	}

	// Skip orders that were already validated (redelivery)
	processed, err := s.ProductRepo.GetProcessedValOrdEvent(ctx, eventDTO.OrderID)
	if err != nil {
		return err
	}
	if processed {
		return nil
	}

	err = s.ProductRepo.ReserveInventory(ctx, eventDTO.OrderID, toItemQuantities(eventDTO.Items))
	switch {
	case err == nil:
		return nil
	case errors.Is(err, repository.ErrInsufficientInventory):
		// Business failure: record it so order-service marks the order FAILED. Not retried.
		s.ZapLogger.Info("not enough inventory for order", zap.Uint64("order_id", eventDTO.OrderID))
		return s.ProductRepo.RecordFailedValidation(ctx, eventDTO.OrderID)
	default:
		// Technical failure (database): let the consumer retry
		s.ZapLogger.Error("failed to reserve inventory", zap.Uint64("order_id", eventDTO.OrderID), zap.Error(err))
		return err
	}
}

// ReleaseProductInventory gives inventory back when order-service cancels a confirmed order.
func (s *ProductService) ReleaseProductInventory(ctx context.Context, msg *kafka.Message) error {
	var eventDTO dto.CancelOrderKafkaEvent
	if err := json.Unmarshal(msg.Value, &eventDTO); err != nil {
		s.ZapLogger.Error("failed to unmarshal cancel event", zap.Error(err))
		return nil
	}
	return s.ProductRepo.ReleaseInventory(ctx, eventDTO.OrderID, toItemQuantities(eventDTO.Items))
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
	eventsModel, err := s.ProductRepo.GetValOrdEventNotPublish(ctx, limit)
	if err != nil {
		return err
	}

	var firstErr error
	for _, eventModel := range eventsModel {
		if err := s.producerValOrdKafkaEvent(ctx, eventModel, topic); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *ProductService) producerValOrdKafkaEvent(ctx context.Context, eventModel *outbox.ValidateOrderKafkaEvent, topic string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	eventJson, err := json.Marshal(eventModel)
	if err != nil {
		return err
	}
	// Key = order ID keeps all events of an order on one partition, in order.
	key := []byte(strconv.FormatUint(eventModel.OrderID, 10))
	if err := s.MQProducer.Publish(ctx, &kafka.Hash{}, topic, key, eventJson); err != nil {
		s.ZapLogger.Warn("ProductService: publish to Kafka failure", zap.Error(err))
		if err2 := s.ProductRepo.UpdateValOrdEventStatus(ctx, eventModel.OrderID, "FAILED"); err2 != nil {
			s.ZapLogger.Warn("ProductService: publish to Kafka failure and can not update OutboxDB")
			return err2
		}
		return err
	}
	// A crash before this update re-publishes the event; order-service handles duplicates.
	if err := s.ProductRepo.UpdateValOrdEventStatus(ctx, eventModel.OrderID, "SUCCESS"); err != nil {
		s.ZapLogger.Warn("ProductService: publish to Kafka success but update to OutboxDB failed")
		return err
	}
	return nil
}
