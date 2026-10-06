package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"payment-service/pkg/events"
	"payment-service/pkg/model"
	"strings"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// View is a payment with the optional admin-only details.
type View struct {
	Payment    *model.Payment
	NextAction string
	Attempts   []*model.Attempt
	Refunds    []*model.Refund
	OrderIDs   []uint64
}

// nextAction tells the UI what the buyer has to do.
func nextAction(p *model.Payment) string {
	switch p.Status {
	case model.StatusRequiresAction:
		switch p.Method {
		case model.MethodCard:
			if p.ThreeDSPassed {
				return "otp"
			}
			return "enter_card"
		case model.MethodWallet:
			return "approve_in_wallet"
		case model.MethodTransfer:
			return "transfer_and_confirm"
		}
	case model.StatusProcessing:
		return "wait"
	}
	return "none"
}

func (s *PaymentService) viewOf(p *model.Payment, admin bool) *View {
	v := &View{Payment: p, NextAction: nextAction(p)}
	_ = json.Unmarshal(p.OrderIDs, &v.OrderIDs)
	if admin {
		s.DB.Where("payment_id = ?", p.ID).Order("id ASC").Find(&v.Attempts)
		s.DB.Where("payment_id = ?", p.ID).Order("id ASC").Find(&v.Refunds)
	}
	return v
}

// Scope restricts a read to the buyer who owns the payment (admins see everything).
type Scope struct {
	BuyerID uint64
	Admin   bool
}

func (sc Scope) apply(q *gorm.DB) *gorm.DB {
	if sc.Admin {
		return q
	}
	return q.Where("buyer_id = ? AND ? <> 0", sc.BuyerID, sc.BuyerID)
}

// GetPayment returns a payment in scope (NotFound otherwise).
func (s *PaymentService) GetPayment(ctx context.Context, id uint64, sc Scope) (*View, error) {
	var p model.Payment
	if err := sc.apply(s.DB.WithContext(ctx).Where("id = ?", id)).First(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "payment not found")
		}
		return nil, err
	}
	return s.viewOf(&p, sc.Admin), nil
}

// GetByCheckout returns the payment of a checkout in scope.
func (s *PaymentService) GetByCheckout(ctx context.Context, checkoutID string, sc Scope) (*View, error) {
	var p model.Payment
	if err := sc.apply(s.DB.WithContext(ctx).Where("checkout_id = ?", checkoutID)).First(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "payment not found")
		}
		return nil, err
	}
	return s.viewOf(&p, sc.Admin), nil
}

// List returns a page of payments (admin view), newest first.
func (s *PaymentService) List(ctx context.Context, statusFilter, method string, page, pageSize uint64) ([]*View, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := s.DB.WithContext(ctx).Model(&model.Payment{})
	if statusFilter != "" {
		q = q.Where("status = ?", strings.ToUpper(statusFilter))
	}
	if method != "" {
		q = q.Where("method = ?", strings.ToUpper(method))
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var ps []*model.Payment
	if err := q.Order("id DESC").Limit(int(pageSize)).Offset(int((page - 1) * pageSize)).Find(&ps).Error; err != nil {
		return nil, 0, err
	}
	out := make([]*View, 0, len(ps))
	for _, p := range ps {
		out = append(out, s.viewOf(p, false))
	}
	return out, total, nil
}

// ---- refunds -------------------------------------------------------------------------------------

// RefundInput describes a refund. Exactly one of PaymentID / CheckoutID identifies the payment.
type RefundInput struct {
	PaymentID   uint64
	CheckoutID  string
	OrderID     uint64
	AmountMinor int64
	Reason      string
	Key         string // idempotency key: a key is refunded once
}

// Refund returns money to the buyer (simulated: succeeds immediately). It is idempotent per Key, never refunds more
// than was paid, and only applies to payments that actually succeeded. Returns the refund row, or nil when the
// request was a duplicate or did not apply.
func (s *PaymentService) Refund(ctx context.Context, in RefundInput) (*model.Refund, error) {
	if in.AmountMinor <= 0 || in.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "amount and key are required")
	}
	var refund *model.Refund
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p model.Payment
		q := tx.Clauses(clause.Locking{Strength: "UPDATE"})
		if in.PaymentID != 0 {
			q = q.Where("id = ?", in.PaymentID)
		} else {
			q = q.Where("checkout_id = ?", in.CheckoutID)
		}
		if err := q.First(&p).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				s.Logger.Info("refund requested for a checkout without payment", zap.String("checkout", in.CheckoutID))
				return nil
			}
			return err
		}
		if p.Status != model.StatusSucceeded && p.Status != model.StatusPartialRefunded {
			s.Logger.Info("refund ignored: payment was not paid", zap.Uint64("payment", p.ID), zap.String("status", p.Status))
			return nil
		}
		r := &model.Refund{PaymentID: p.ID, OrderID: in.OrderID, RefundKey: in.Key, AmountMinor: in.AmountMinor, Reason: in.Reason, Status: model.RefundSucceeded}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(r)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil // duplicate request
		}
		if in.AmountMinor > p.AmountMinor-p.RefundedMinor {
			r.Status = model.RefundFailed
			r.Reason = "exceeds_paid_amount"
			refund = r
			return tx.Model(r).Updates(map[string]any{"status": r.Status, "reason": r.Reason}).Error
		}
		p.RefundedMinor += in.AmountMinor
		p.Status = model.StatusPartialRefunded
		if p.RefundedMinor == p.AmountMinor {
			p.Status = model.StatusRefunded
		}
		if err := tx.Model(&p).Updates(map[string]any{"refunded_minor": p.RefundedMinor, "status": p.Status}).Error; err != nil {
			return err
		}
		refund = r
		return events.Emit(tx, TopicRefunded, pkey(p.ID), refundedEvent{
			V: 1, PaymentID: p.ID, RefundID: r.ID, CheckoutID: p.CheckoutID, OrderID: in.OrderID, AmountMinor: in.AmountMinor, Reason: in.Reason, At: s.Now().UTC(),
		})
	})
	return refund, err
}

// AdminRefund refunds part or all of a payment on behalf of an administrator.
func (s *PaymentService) AdminRefund(ctx context.Context, paymentID uint64, amountMinor int64, reason string) (*View, error) {
	r, err := s.Refund(ctx, RefundInput{PaymentID: paymentID, AmountMinor: amountMinor, Reason: strings.TrimSpace(reason), Key: fmt.Sprintf("admin:%d:%d", paymentID, s.Now().UnixNano())})
	if err != nil {
		return nil, err
	}
	if r == nil || r.Status != model.RefundSucceeded {
		return nil, status.Error(codes.FailedPrecondition, "the payment can not be refunded by that amount")
	}
	return s.GetPayment(ctx, paymentID, Scope{Admin: true})
}

// ForceStatus applies a provider outcome immediately. Only available with ENV=dev.
func (s *PaymentService) ForceStatus(ctx context.Context, paymentID uint64, to string) (*View, error) {
	if !s.Cfg.Dev {
		return nil, status.Error(codes.PermissionDenied, "forcing a payment status is only available in development")
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := lockPayment(tx, paymentID)
		if err != nil {
			return err
		}
		switch strings.ToUpper(to) {
		case model.StatusSucceeded:
			_, err = s.succeed(tx, p, fmt.Sprintf("evt_%d_force_%d", p.ID, s.Now().UnixNano()))
			return err
		case model.StatusFailed:
			p.FailedCount = model.MaxFailedAttempts - 1
			return s.fail(tx, p, "forced", "forced_failure")
		case model.StatusExpired:
			p.Status = model.StatusExpired
			return tx.Model(p).Update("status", p.Status).Error
		}
		return status.Error(codes.InvalidArgument, "status must be SUCCEEDED, FAILED or EXPIRED")
	})
	if err != nil {
		return nil, err
	}
	return s.GetPayment(ctx, paymentID, Scope{Admin: true})
}

// ---- Kafka consumers -------------------------------------------------------------------------------

// HandlePaymentRequested creates the payment of a checkout when order-service asks for it (idempotent).
func (s *PaymentService) HandlePaymentRequested(ctx context.Context, msg *kafka.Message) error {
	var req paymentRequested
	if err := json.Unmarshal(msg.Value, &req); err != nil {
		s.Logger.Error("invalid payment.requested event", zap.Error(err))
		return nil
	}
	if _, _, err := s.CreatePayment(ctx, req); err != nil {
		if status.Code(err) == codes.InvalidArgument {
			s.Logger.Error("rejected payment.requested event", zap.Error(err), zap.String("checkout", req.CheckoutID))
			return nil
		}
		return err
	}
	return nil
}

type refundRequested struct {
	CheckoutID  string `json:"checkout_id"`
	OrderID     uint64 `json:"order_id"`
	AmountMinor int64  `json:"amount_minor"`
	Reason      string `json:"reason"`
	RefundKey   string `json:"refund_key"`
}

// HandleRefundRequested refunds money when order-service asks for it (idempotent per refund key).
func (s *PaymentService) HandleRefundRequested(ctx context.Context, msg *kafka.Message) error {
	var req refundRequested
	if err := json.Unmarshal(msg.Value, &req); err != nil || req.CheckoutID == "" {
		s.Logger.Error("invalid order.refund_requested event", zap.Error(err))
		return nil
	}
	_, err := s.Refund(ctx, RefundInput{CheckoutID: req.CheckoutID, OrderID: req.OrderID, AmountMinor: req.AmountMinor, Reason: req.Reason, Key: req.RefundKey})
	if status.Code(err) == codes.InvalidArgument {
		s.Logger.Error("rejected refund request", zap.Error(err))
		return nil
	}
	return err
}
