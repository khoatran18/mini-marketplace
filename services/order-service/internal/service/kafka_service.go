package service

import (
	"context"
	"encoding/json"
	"order-service/internal/service/adapter"
	"order-service/pkg/dto"
	"order-service/pkg/model"
	"order-service/pkg/outbox"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// publishTimeout bounds one publish + outbox status update.
const publishTimeout = 5 * time.Second

// For Consumer

// UpdateOrderStatusByKafka applies the inventory validation result of product-service.
// It is idempotent: a PENDING order is moved to SUCCESS/FAILED once, and replays or
// out-of-order events find no PENDING row and are ignored.
func (s *OrderService) UpdateOrderStatusByKafka(ctx context.Context, msg *kafka.Message) error {
	var eventDTO dto.ValidateOrderKafkaEvent
	if err := json.Unmarshal(msg.Value, &eventDTO); err != nil {
		// A malformed message can never succeed; drop it instead of retrying.
		s.ZapLogger.Error("OrderService: invalid validate-order event", zap.Error(err))
		return nil
	}

	target := model.StatusSuccess
	if !eventDTO.Success {
		target = model.StatusFailed
	}
	changed, err := s.OrderRepo.TransitionStatus(ctx, eventDTO.OrderID, []string{model.StatusPending}, target)
	if err != nil {
		return err
	}
	if !changed {
		s.ZapLogger.Info("OrderService: validate-order event ignored (order missing or not pending)",
			zap.Uint64("order_id", eventDTO.OrderID), zap.String("target", target))
	}
	return nil
}

// For Producer

// runOutboxWorker calls batch every interval until ctx is canceled.
func (s *OrderService) runOutboxWorker(ctx context.Context, name string, interval time.Duration, batch func(context.Context) error) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				s.ZapLogger.Info("OrderService: outbox worker stopped", zap.String("worker", name))
				return
			case <-ticker.C:
				if err := batch(ctx); err != nil {
					s.ZapLogger.Warn("OrderService: outbox batch error", zap.String("worker", name), zap.Error(err))
				}
			}
		}
	}()
}

// ProducerCreOrdKafkaEventWorker publishes CreateOrder outbox events.
func (s *OrderService) ProducerCreOrdKafkaEventWorker(ctx context.Context, interval time.Duration, limit int, topic string) {
	s.runOutboxWorker(ctx, "create_order", interval, func(ctx context.Context) error {
		return s.producerCreOrdKafkaEventBatch(ctx, limit, topic)
	})
}

func (s *OrderService) producerCreOrdKafkaEventBatch(ctx context.Context, limit int, topic string) error {
	eventsModel, err := s.OrderRepo.GetCreateOrderEventNotPublish(ctx, limit)
	if err != nil {
		return err
	}

	var firstErr error
	for _, eventModel := range eventsModel {
		eventKafka, err := adapter.CreOrdEvesModelToKafkaEvent(eventModel)
		if err != nil {
			s.ZapLogger.Error("OrderService: can not build create-order event", zap.Uint64("order_id", eventModel.OrderID), zap.Error(err))
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := s.publishCreateOrder(ctx, eventKafka, topic); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *OrderService) publishCreateOrder(ctx context.Context, event *outbox.CreateOrderKafkaEvent, topic string) error {
	ctx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	// Key = order ID: events of one order always land on the same partition, in order.
	key := []byte(strconv.FormatUint(event.OrderID, 10))
	if err := s.MQProducer.Publish(ctx, &kafka.Hash{}, topic, key, payload); err != nil {
		s.ZapLogger.Warn("OrderService: publish create-order failed", zap.Uint64("order_id", event.OrderID), zap.Error(err))
		if err2 := s.OrderRepo.UpdateCreateOrderEventStatus(ctx, event.OrderID, "FAILED"); err2 != nil {
			s.ZapLogger.Warn("OrderService: publish failed and can not update outbox", zap.Error(err2))
		}
		return err
	}
	// A crash between publish and this update re-publishes the event; consumers are idempotent.
	if err := s.OrderRepo.UpdateCreateOrderEventStatus(ctx, event.OrderID, "SUCCESS"); err != nil {
		s.ZapLogger.Warn("OrderService: published create-order but outbox update failed", zap.Error(err))
		return err
	}
	return nil
}

// ProducerCancelOrdKafkaEventWorker publishes CancelOrder outbox events so inventory is released.
func (s *OrderService) ProducerCancelOrdKafkaEventWorker(ctx context.Context, interval time.Duration, limit int, topic string) {
	s.runOutboxWorker(ctx, "cancel_order", interval, func(ctx context.Context) error {
		events, err := s.OrderRepo.GetCancelOrderEventNotPublish(ctx, limit)
		if err != nil {
			return err
		}
		var firstErr error
		for _, e := range events {
			if err := s.publishCancelOrder(ctx, e, topic); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	})
}

func (s *OrderService) publishCancelOrder(ctx context.Context, e *outbox.CancelOrderEvent, topic string) error {
	ctx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()

	event := &outbox.CancelOrderKafkaEvent{OrderID: e.OrderID}
	if err := json.Unmarshal(e.Items, &event.Items); err != nil {
		s.ZapLogger.Error("OrderService: can not decode cancel-order items", zap.Uint64("order_id", e.OrderID), zap.Error(err))
		return err
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	key := []byte(strconv.FormatUint(e.OrderID, 10))
	if err := s.MQProducer.Publish(ctx, &kafka.Hash{}, topic, key, payload); err != nil {
		s.ZapLogger.Warn("OrderService: publish cancel-order failed", zap.Uint64("order_id", e.OrderID), zap.Error(err))
		if err2 := s.OrderRepo.UpdateCancelOrderEventStatus(ctx, e.OrderID, "FAILED"); err2 != nil {
			s.ZapLogger.Warn("OrderService: publish failed and can not update outbox", zap.Error(err2))
		}
		return err
	}
	return s.OrderRepo.UpdateCancelOrderEventStatus(ctx, e.OrderID, "SUCCESS")
}
