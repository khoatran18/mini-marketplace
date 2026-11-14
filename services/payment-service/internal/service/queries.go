package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mini-marketplace/shared/events"
	"payment-service/pkg/model"
	"strings"
	"time"

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

// ListFilter narrows the payment list. Every field is optional: the zero value means "no filter".
type ListFilter struct {
	Status         string
	Method         string
	CreatedFrom    time.Time // inclusive
	CreatedTo      time.Time // exclusive
	MinAmountMinor int64
	MaxAmountMinor int64
	Sort           string // newest (default) | oldest | amount_desc | amount_asc
}

// List returns a page of payments (admin), optionally by status and method.
func (s *PaymentService) List(ctx context.Context, statusFilter, method string, page, pageSize uint64) ([]*View, int64, error) {
	return s.ListFiltered(ctx, ListFilter{Status: statusFilter, Method: method}, page, pageSize)
}

// ListFiltered returns a page of payments that match f. Unknown values are rejected (InvalidArgument) instead of silently matching nothing.
func (s *PaymentService) ListFiltered(ctx context.Context, f ListFilter, page, pageSize uint64) ([]*View, int64, error) {
	if f.Status != "" && !model.ValidStatus(strings.ToUpper(f.Status)) {
		return nil, 0, status.Error(codes.InvalidArgument, "unknown status")
	}
	if f.Method != "" && !model.ValidMethod(strings.ToUpper(f.Method)) {
		return nil, 0, status.Error(codes.InvalidArgument, "unknown method")
	}
	order := "id DESC"
	switch f.Sort {
	case "", "newest":
	case "oldest":
		order = "id ASC"
	case "amount_desc":
		order = "amount_minor DESC, id DESC"
	case "amount_asc":
		order = "amount_minor ASC, id DESC"
	default:
		return nil, 0, status.Error(codes.InvalidArgument, "unknown sort")
	}
	if f.MinAmountMinor < 0 || f.MaxAmountMinor < 0 || (f.MaxAmountMinor > 0 && f.MinAmountMinor > f.MaxAmountMinor) {
		return nil, 0, status.Error(codes.InvalidArgument, "min_amount must not be greater than max_amount")
	}
	if !f.CreatedFrom.IsZero() && !f.CreatedTo.IsZero() && !f.CreatedFrom.Before(f.CreatedTo) {
		return nil, 0, status.Error(codes.InvalidArgument, "created_from must be before created_to")
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := s.DB.WithContext(ctx).Model(&model.Payment{})
	if f.Status != "" {
		q = q.Where("status = ?", strings.ToUpper(f.Status))
	}
	if f.Method != "" {
		q = q.Where("method = ?", strings.ToUpper(f.Method))
	}
	if !f.CreatedFrom.IsZero() {
		q = q.Where("created_at >= ?", f.CreatedFrom)
	}
	if !f.CreatedTo.IsZero() {
		q = q.Where("created_at < ?", f.CreatedTo)
	}
	if f.MinAmountMinor > 0 {
		q = q.Where("amount_minor >= ?", f.MinAmountMinor)
	}
	if f.MaxAmountMinor > 0 {
		q = q.Where("amount_minor <= ?", f.MaxAmountMinor)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var ps []*model.Payment
	if err := q.Order(order).Limit(int(pageSize)).Offset(int((page - 1) * pageSize)).Find(&ps).Error; err != nil {
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
				s.Metrics.refund("ignored")
				return nil
			}
			return err
		}
		if p.Status != model.StatusSucceeded && p.Status != model.StatusPartialRefunded {
			s.Logger.Info("refund ignored: payment was not paid", zap.Uint64("payment", p.ID), zap.String("status", p.Status))
			s.Metrics.refund("ignored")
			return nil
		}
		r := &model.Refund{PaymentID: p.ID, OrderID: in.OrderID, RefundKey: in.Key, AmountMinor: in.AmountMinor, Reason: in.Reason, Status: model.RefundSucceeded}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(r)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			s.Metrics.refund("duplicate")
			return nil // duplicate request
		}
		if in.AmountMinor > p.AmountMinor-p.RefundedMinor {
			r.Status = model.RefundFailed
			r.Reason = "exceeds_paid_amount"
			refund = r
			s.Metrics.refund("failed")
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
		s.Metrics.refund("succeeded")
		return events.Emit(tx, TopicRefunded, pkey(p.ID), refundedEvent{
			V: 1, PaymentID: p.ID, RefundID: r.ID, CheckoutID: p.CheckoutID, OrderID: in.OrderID, AmountMinor: in.AmountMinor, Reason: in.Reason, At: s.Now().UTC(), PaymentStatus: p.Status,
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
			s.Metrics.paymentCompleted(p.Method, "expired", 0, p.CreatedAt, s.Now())
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
