package service

import (
	"context"
	"encoding/json"
	"errors"
	"order-service/internal/repository"
	"order-service/pkg/model"
	"order-service/pkg/outbox"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// publishTimeout bounds one publish + outbox status update.
const publishTimeout = 5 * time.Second

// ignorable reports errors that mean "this event no longer applies" (replay, out-of-order, unknown order).
func ignorable(err error) bool {
	return errors.Is(err, repository.ErrNotAllowed) || errors.Is(err, repository.ErrOrderNotFound)
}

// ---- consumers --------------------------------------------------------------------------------

// validateResultEvent is the payload of product.validate_order.
type validateResultEvent struct {
	OrderID uint64 `json:"order_id"`
	Success bool   `json:"success"`
}

// UpdateOrderStatusByKafka applies the stock reservation result of product-service to a PENDING order:
// online methods wait for payment (with a deadline), cash on delivery is confirmed, no stock means FAILED.
// It is idempotent: replays and out-of-order events find the order no longer PENDING and are ignored.
func (s *OrderService) UpdateOrderStatusByKafka(ctx context.Context, msg *kafka.Message) error {
	var ev validateResultEvent
	if err := json.Unmarshal(msg.Value, &ev); err != nil {
		// A malformed message can never succeed; drop it instead of retrying.
		s.ZapLogger.Error("OrderService: invalid validate-order event", zap.Error(err))
		return nil
	}
	order, _, err := s.OrderRepo.GetOrder(ctx, ev.OrderID, repository.Scope{Admin: true})
	if errors.Is(err, repository.ErrOrderNotFound) {
		s.ZapLogger.Info("OrderService: validate-order event for an unknown order", zap.Uint64("order_id", ev.OrderID))
		return nil
	}
	if err != nil {
		return err
	}
	p := repository.TransitionParams{Actor: repository.Actor{Type: repository.ActorSystem}}
	switch {
	case !ev.Success:
		p.To, p.Reason = model.StatusFailed, "out_of_stock"
	case model.IsOnline(order.PaymentMethod):
		p.To, p.PaymentTTL = model.StatusAwaitingPayment, s.Settings.PaymentTTL
	default:
		p.To = model.StatusConfirmed
	}
	if _, err := s.OrderRepo.Transition(ctx, ev.OrderID, p); err != nil {
		if ignorable(err) {
			s.ZapLogger.Info("OrderService: validate-order event ignored", zap.Uint64("order_id", ev.OrderID), zap.Error(err))
			return nil
		}
		return err
	}
	return nil
}

// paymentSucceededEvent is the payload of payment.succeeded.
type paymentSucceededEvent struct {
	CheckoutID  string `json:"checkout_id"`
	PaymentID   uint64 `json:"payment_id"`
	AmountMinor int64  `json:"amount_minor"`
}

// HandlePaymentSucceeded marks every order of the checkout that is still awaiting payment as PAID. Orders that
// can no longer be paid (canceled or expired meanwhile) are not charged: the difference is refunded.
func (s *OrderService) HandlePaymentSucceeded(ctx context.Context, msg *kafka.Message) error {
	var ev paymentSucceededEvent
	if err := json.Unmarshal(msg.Value, &ev); err != nil || ev.CheckoutID == "" {
		s.ZapLogger.Error("OrderService: invalid payment.succeeded event", zap.Error(err))
		return nil
	}
	orders, err := s.OrderRepo.OrdersInCheckout(ctx, ev.CheckoutID)
	if err != nil {
		return err
	}
	if len(orders) == 0 {
		return nil
	}
	for _, o := range orders {
		if o.Status != model.StatusAwaitingPayment {
			continue
		}
		if _, err := s.OrderRepo.Transition(ctx, o.ID, repository.TransitionParams{To: model.StatusPaid, Actor: repository.Actor{Type: repository.ActorPayment}}); err != nil && !ignorable(err) {
			return err
		}
	}
	// What was really paid is read back after the transitions: a concurrent delivery of the same event may have
	// done the work (our Transition then reports "not allowed"), and orders canceled after being paid still
	// count as paid (they are refunded through their own cancel path).
	orders, err = s.OrderRepo.OrdersInCheckout(ctx, ev.CheckoutID)
	if err != nil {
		return err
	}
	var paidMinor int64
	for _, o := range orders {
		if o.PaidAt != nil {
			paidMinor += toMinor(o.TotalPrice)
		}
	}
	if over := ev.AmountMinor - paidMinor; over > 0 {
		// money arrived for orders that were canceled/expired in the meantime
		return s.OrderRepo.RequestRefund(ctx, orders[0], over, "order_not_payable", "checkout:"+ev.CheckoutID+":overpay")
	}
	return nil
}

// paymentRefundedEvent is the payload of payment.refunded.
type paymentRefundedEvent struct {
	OrderID uint64 `json:"order_id"`
}

// HandlePaymentRefunded records that the money of an order went back to the buyer.
func (s *OrderService) HandlePaymentRefunded(ctx context.Context, msg *kafka.Message) error {
	var ev paymentRefundedEvent
	if err := json.Unmarshal(msg.Value, &ev); err != nil {
		s.ZapLogger.Error("OrderService: invalid payment.refunded event", zap.Error(err))
		return nil
	}
	if ev.OrderID == 0 {
		return nil
	}
	return s.OrderRepo.MarkPaymentRefunded(ctx, ev.OrderID)
}

// ---- time based workers -----------------------------------------------------------------------

// ExpireUnpaid moves orders whose payment deadline passed to EXPIRED (their stock is released through the outbox).
// Returns how many orders expired.
func (s *OrderService) ExpireUnpaid(ctx context.Context, now time.Time) (int, error) {
	ids, err := s.OrderRepo.DueForExpiry(ctx, now, 100)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		_, err := s.OrderRepo.Transition(ctx, id, repository.TransitionParams{To: model.StatusExpired, Actor: repository.Actor{Type: repository.ActorSystem}, Now: now})
		switch {
		case err == nil:
			n++
		case ignorable(err):
		default:
			return n, err
		}
	}
	return n, nil
}

// AutoDeliver marks orders as delivered when they were shipped more than AutoDeliverAfter ago (0 = disabled).
func (s *OrderService) AutoDeliver(ctx context.Context, now time.Time) (int, error) {
	if s.Settings.AutoDeliverAfter <= 0 {
		return 0, nil
	}
	ids, err := s.OrderRepo.DueForAutoDelivery(ctx, now.Add(-s.Settings.AutoDeliverAfter), 100)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		_, err := s.OrderRepo.Transition(ctx, id, repository.TransitionParams{To: model.StatusDelivered, Actor: repository.Actor{Type: repository.ActorSystem}, Reason: "auto_delivered", Now: now})
		switch {
		case err == nil:
			n++
		case ignorable(err):
		default:
			return n, err
		}
	}
	return n, nil
}

// RunTimers runs ExpireUnpaid and AutoDeliver every interval until ctx is canceled; heartbeat (optional) is
// called after every round so readiness can see the worker is alive.
func (s *OrderService) RunTimers(ctx context.Context, interval time.Duration, heartbeat func()) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				now := time.Now()
				if n, err := s.ExpireUnpaid(ctx, now); err != nil {
					s.ZapLogger.Warn("OrderService: expiry round failed", zap.Error(err))
				} else if n > 0 {
					s.ZapLogger.Info("OrderService: unpaid orders expired", zap.Int("count", n))
				}
				if _, err := s.AutoDeliver(ctx, now); err != nil {
					s.ZapLogger.Warn("OrderService: auto-deliver round failed", zap.Error(err))
				}
				if heartbeat != nil {
					heartbeat()
				}
			}
		}
	}()
}

// ---- legacy outbox drain ----------------------------------------------------------------------
// Rows written by versions before the generic domain_events outbox are still published so that no
// in-flight order is lost during an upgrade.

func (s *OrderService) runLegacyWorker(ctx context.Context, interval time.Duration, round func(context.Context) error) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := round(ctx); err != nil {
					s.ZapLogger.Warn("OrderService: legacy outbox round failed", zap.Error(err))
				}
			}
		}
	}()
}

// ProducerCreOrdKafkaEventWorker publishes legacy create_order_events rows.
func (s *OrderService) ProducerCreOrdKafkaEventWorker(ctx context.Context, interval time.Duration, limit int, topic string) {
	s.runLegacyWorker(ctx, interval, func(ctx context.Context) error {
		rows, err := s.OrderRepo.GetCreateOrderEventNotPublish(ctx, limit)
		if err != nil {
			return err
		}
		for _, e := range rows {
			var items []*outbox.ItemEvent
			if err := json.Unmarshal(e.Items, &items); err != nil {
				continue
			}
			payload, _ := json.Marshal(&outbox.CreateOrderKafkaEvent{OrderID: e.OrderID, Items: items})
			if err := s.publishLegacy(ctx, topic, e.OrderID, payload); err != nil {
				_ = s.OrderRepo.UpdateCreateOrderEventStatus(ctx, e.OrderID, "FAILED")
				return err
			}
			if err := s.OrderRepo.UpdateCreateOrderEventStatus(ctx, e.OrderID, "SUCCESS"); err != nil {
				return err
			}
		}
		return nil
	})
}

// ProducerCancelOrdKafkaEventWorker publishes legacy cancel_order_events rows.
func (s *OrderService) ProducerCancelOrdKafkaEventWorker(ctx context.Context, interval time.Duration, limit int, topic string) {
	s.runLegacyWorker(ctx, interval, func(ctx context.Context) error {
		rows, err := s.OrderRepo.GetCancelOrderEventNotPublish(ctx, limit)
		if err != nil {
			return err
		}
		for _, e := range rows {
			ev := &outbox.CancelOrderKafkaEvent{OrderID: e.OrderID}
			if err := json.Unmarshal(e.Items, &ev.Items); err != nil {
				continue
			}
			payload, _ := json.Marshal(ev)
			if err := s.publishLegacy(ctx, topic, e.OrderID, payload); err != nil {
				_ = s.OrderRepo.UpdateCancelOrderEventStatus(ctx, e.OrderID, "FAILED")
				return err
			}
			if err := s.OrderRepo.UpdateCancelOrderEventStatus(ctx, e.OrderID, "SUCCESS"); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *OrderService) publishLegacy(ctx context.Context, topic string, orderID uint64, payload []byte) error {
	ctx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	return s.MQProducer.Publish(ctx, &kafka.Hash{}, topic, []byte(strconv.FormatUint(orderID, 10)), payload)
}
