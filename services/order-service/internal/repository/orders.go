package repository

import (
	"context"
	"errors"
	"order-service/pkg/events"
	"order-service/pkg/model"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// OrderRepository persists orders, checkouts and carts.
type OrderRepository struct {
	DB *gorm.DB
}

// NewOrderRepository creates the repository.
func NewOrderRepository(db *gorm.DB) *OrderRepository { return &OrderRepository{DB: db} }

// MaxPageSize caps list sizes.
const MaxPageSize = 100

// ---- checkout creation --------------------------------------------------------------------

// CheckoutCreate is a fully priced checkout ready to be stored.
type CheckoutCreate struct {
	Checkout   *model.Checkout
	Orders     []*model.Order // one per store, items attached
	CartClear  uint64         // user whose cart lines for the ordered products are removed (0 = none)
	CartLines  []uint64       // product ids to remove from that cart
	ActorBuyer uint64
}

// CreateCheckout stores the checkout, its orders, the initial history rows and the stock-reservation events in
// one transaction. When the (buyer, idempotency key) pair already exists nothing is written and the existing
// checkout is returned with replayed=true.
func (r *OrderRepository) CreateCheckout(ctx context.Context, in *CheckoutCreate) (checkout *model.Checkout, orders []*model.Order, replayed bool, err error) {
	err = r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(in.Checkout)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			replayed = true
			return nil
		}
		for _, o := range in.Orders {
			o.CheckoutID = in.Checkout.ID
			if err := tx.Create(o).Error; err != nil {
				return err
			}
			if err := tx.Create(&model.OrderStatusHistory{OrderID: o.ID, ToStatus: model.StatusPending, ActorType: ActorBuyer, ActorID: in.ActorBuyer}).Error; err != nil {
				return err
			}
			if err := emitStatusChanged(tx, o, "", ActorBuyer); err != nil {
				return err
			}
			if err := emitCreateOrder(tx, o); err != nil {
				return err
			}
		}
		if in.CartClear != 0 && len(in.CartLines) > 0 {
			return tx.Where("user_id = ? AND product_id IN ?", in.CartClear, in.CartLines).Delete(&model.CartItem{}).Error
		}
		return nil
	})
	if err != nil {
		return nil, nil, false, err
	}
	if replayed {
		existing, orders, err := r.CheckoutByKey(ctx, in.Checkout.BuyerID, in.Checkout.IdempotencyKey)
		return existing, orders, true, err
	}
	return in.Checkout, in.Orders, false, nil
}

// CheckoutByKey loads a checkout by its idempotency key together with its orders.
func (r *OrderRepository) CheckoutByKey(ctx context.Context, buyerID uint64, key string) (*model.Checkout, []*model.Order, error) {
	var c model.Checkout
	if err := r.DB.WithContext(ctx).Where("buyer_id = ? AND idempotency_key = ?", buyerID, key).First(&c).Error; err != nil {
		return nil, nil, err
	}
	orders, err := r.ordersOfCheckout(ctx, c.ID)
	return &c, orders, err
}

// GetCheckout returns a buyer's checkout with its orders.
func (r *OrderRepository) GetCheckout(ctx context.Context, buyerID uint64, id string) (*model.Checkout, []*model.Order, error) {
	var c model.Checkout
	if err := r.DB.WithContext(ctx).Where("id = ? AND buyer_id = ?", id, buyerID).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrOrderNotFound
		}
		return nil, nil, err
	}
	orders, err := r.ordersOfCheckout(ctx, c.ID)
	return &c, orders, err
}

func (r *OrderRepository) ordersOfCheckout(ctx context.Context, checkoutID string) ([]*model.Order, error) {
	var orders []*model.Order
	err := r.DB.WithContext(ctx).Preload("OrderItems").Where("checkout_id = ?", checkoutID).Order("id ASC").Find(&orders).Error
	return orders, err
}

// ---- reads -----------------------------------------------------------------------------------

// Scope restricts which orders a caller may see.
type Scope struct {
	BuyerID uint64
	StoreID uint64
	Admin   bool
}

func (s Scope) valid() bool { return s.Admin || s.BuyerID != 0 || s.StoreID != 0 }

func (s Scope) apply(q *gorm.DB) *gorm.DB {
	switch {
	case s.Admin:
		return q
	case s.StoreID != 0:
		return q.Where("store_id = ?", s.StoreID)
	default:
		return q.Where("buyer_id = ?", s.BuyerID)
	}
}

// GetOrder returns one order with items and history, only when it is inside the caller's scope.
func (r *OrderRepository) GetOrder(ctx context.Context, id uint64, scope Scope) (*model.Order, []*model.OrderStatusHistory, error) {
	if !scope.valid() {
		return nil, nil, ErrOrderNotFound
	}
	var o model.Order
	q := scope.apply(r.DB.WithContext(ctx).Preload("OrderItems").Where("id = ?", id))
	if err := q.First(&o).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrOrderNotFound
		}
		return nil, nil, err
	}
	var history []*model.OrderStatusHistory
	if err := r.DB.WithContext(ctx).Where("order_id = ?", id).Order("id ASC").Find(&history).Error; err != nil {
		return nil, nil, err
	}
	return &o, history, nil
}

// ListOrders returns a page of orders in scope (newest first) and the total count.
func (r *OrderRepository) ListOrders(ctx context.Context, scope Scope, status string, page, pageSize uint64) ([]*model.Order, int64, error) {
	if !scope.valid() {
		return nil, 0, nil
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > MaxPageSize {
		pageSize = 20
	}
	q := scope.apply(r.DB.WithContext(ctx).Model(&model.Order{}))
	if status != "" {
		q = q.Where("status = ?", strings.ToUpper(status))
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var orders []*model.Order
	err := q.Preload("OrderItems").Order("id DESC").Limit(int(pageSize)).Offset(int((page - 1) * pageSize)).Find(&orders).Error
	return orders, total, err
}

// CountOrders returns the number of orders per status in scope.
func (r *OrderRepository) CountOrders(ctx context.Context, scope Scope) (map[string]int64, error) {
	out := map[string]int64{}
	if !scope.valid() {
		return out, nil
	}
	var rows []struct {
		Status string
		N      int64
	}
	if err := scope.apply(r.DB.WithContext(ctx).Model(&model.Order{})).Select("status, count(*) AS n").Group("status").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.Status] = row.N
	}
	return out, nil
}

// ---- transitions -----------------------------------------------------------------------------

// TransitionParams describes one status change.
type TransitionParams struct {
	To           string
	Actor        Actor
	Reason       string
	Carrier      string
	TrackingCode string
	ReturnWindow time.Duration // how long after delivery a return may be requested (REFUND_REQUESTED)
	PaymentTTL   time.Duration // unpaid deadline starting now (AWAITING_PAYMENT)
	Now          time.Time     // zero = time.Now()
}

// Transition moves an order to p.To when the state machine and ownership rules allow it, in one transaction that
// also writes the history row and every event (status change, stock release, refund, payment request).
// Buyers and sellers get ErrOrderNotFound for orders that are not theirs.
func (r *OrderRepository) Transition(ctx context.Context, orderID uint64, p TransitionParams) (*model.Order, error) {
	var out *model.Order
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var o model.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("OrderItems").Where("id = ?", orderID).First(&o).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOrderNotFound
			}
			return err
		}
		switch p.Actor.Type {
		case ActorBuyer:
			if p.Actor.ID == 0 || o.BuyerID != p.Actor.ID {
				return ErrOrderNotFound
			}
		case ActorSeller:
			if p.Actor.StoreID == 0 || o.StoreID != p.Actor.StoreID {
				return ErrOrderNotFound
			}
		}
		if !CanTransition(o.Status, p.To, p.Actor.Type) {
			return &InvalidTransitionError{From: o.Status, To: p.To, Actor: p.Actor.Type}
		}
		now := p.Now
		if now.IsZero() {
			now = time.Now()
		}
		prev := o.Status
		updates := map[string]interface{}{"status": p.To}
		o.Status = p.To

		stamp := func(col string, field **time.Time) {
			t := now
			updates[col] = t
			*field = &t
		}
		switch p.To {
		case model.StatusAwaitingPayment:
			if p.PaymentTTL > 0 {
				exp := now.Add(p.PaymentTTL)
				o.ExpiresAt = &exp
				updates["expires_at"] = exp
			}
		case model.StatusPaid:
			o.PaymentStatus = model.PaymentPaid
			updates["payment_status"] = model.PaymentPaid
			stamp("paid_at", &o.PaidAt)
		case model.StatusShipped:
			o.Carrier, o.TrackingCode = p.Carrier, p.TrackingCode
			updates["carrier"], updates["tracking_code"] = p.Carrier, p.TrackingCode
			stamp("shipped_at", &o.ShippedAt)
		case model.StatusDelivered:
			if prev == model.StatusRefundRequested { // return rejected: back to delivered, no second request
				o.ReturnRejected = true
				updates["return_rejected"] = true
			} else {
				stamp("delivered_at", &o.DeliveredAt)
				if o.PaymentMethod == model.MethodCOD { // cash collected by the carrier: revenue is recognised now
					o.PaymentStatus = model.PaymentPaid
					updates["payment_status"] = model.PaymentPaid
					stamp("paid_at", &o.PaidAt)
				}
			}
		case model.StatusRefundRequested:
			if o.ReturnRejected {
				return ErrReturnClosed
			}
			if p.ReturnWindow > 0 && (o.DeliveredAt == nil || now.After(o.DeliveredAt.Add(p.ReturnWindow))) {
				return ErrReturnWindow
			}
			o.ReturnReason = p.Reason
			updates["return_reason"] = p.Reason
		case model.StatusRefunded:
			o.PaymentStatus = model.PaymentRefunded
			updates["payment_status"] = model.PaymentRefunded
		case model.StatusCanceled, model.StatusExpired, model.StatusFailed:
			reason := p.Reason
			by := p.Actor.Type
			if p.To == model.StatusExpired {
				reason = "payment_timeout"
			}
			if p.To == model.StatusFailed && reason == "" {
				reason = "out_of_stock"
			}
			o.CancelReason, o.CanceledBy = reason, by
			updates["cancel_reason"], updates["canceled_by"] = reason, by
			stamp("canceled_at", &o.CanceledAt)
			if err := tx.Model(&model.OrderItem{}).Where("order_id = ?", o.ID).Update("status", "CANCELED").Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&model.Order{}).Where("id = ?", o.ID).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.OrderStatusHistory{
			OrderID: o.ID, FromStatus: prev, ToStatus: p.To, ActorType: p.Actor.Type, ActorID: p.Actor.ID, Reason: p.Reason,
		}).Error; err != nil {
			return err
		}
		if err := emitStatusChanged(tx, &o, prev, p.Actor.Type); err != nil {
			return err
		}

		// stock: canceled/expired orders gave their reservation back (a FAILED order never reserved anything)
		if p.To == model.StatusCanceled || p.To == model.StatusExpired {
			if err := emitReleaseStock(tx, &o); err != nil {
				return err
			}
		}
		// money: a paid order that is canceled before shipping, or a return that was approved, is refunded
		if p.To == model.StatusCanceled && o.PaymentStatus == model.PaymentPaid && model.IsOnline(o.PaymentMethod) {
			if err := emitRefund(tx, o.CheckoutID, o.ID, minor(o.TotalPrice), "order_canceled", "order:"+orderKey(o.ID)+":cancel"); err != nil {
				return err
			}
		}
		if p.To == model.StatusRefunded && model.IsOnline(o.PaymentMethod) {
			if err := emitRefund(tx, o.CheckoutID, o.ID, minor(o.TotalPrice), "return_approved", "order:"+orderKey(o.ID)+":return"); err != nil {
				return err
			}
		}
		// the last order of an online checkout leaving PENDING triggers the payment request
		if prev == model.StatusPending {
			if err := requestPaymentIfReady(tx, o.CheckoutID); err != nil {
				return err
			}
		}
		out = &o
		return nil
	})
	return out, err
}

// requestPaymentIfReady emits payment.requested for a checkout when none of its orders is PENDING any more and at
// least one awaits payment. The rows are locked so two validation results arriving together emit it only once.
func requestPaymentIfReady(tx *gorm.DB, checkoutID string) error {
	var orders []model.Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("checkout_id = ?", checkoutID).Order("id ASC").Find(&orders).Error; err != nil {
		return err
	}
	var total int64
	var ids []uint64
	var expires time.Time
	var buyer uint64
	var method string
	for _, o := range orders {
		if o.Status == model.StatusPending {
			return nil // another order of the checkout is still being validated
		}
		if o.Status == model.StatusAwaitingPayment {
			total += minor(o.TotalPrice)
			ids = append(ids, o.ID)
			buyer, method = o.BuyerID, o.PaymentMethod
			if o.ExpiresAt != nil && (expires.IsZero() || o.ExpiresAt.Before(expires)) {
				expires = *o.ExpiresAt
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return events.Emit(tx, TopicPaymentRequest, checkoutID, PaymentRequestedEvent{
		V: 1, CheckoutID: checkoutID, BuyerID: buyer, Method: method, AmountMinor: total, ExpiresAt: expires.UTC(), OrderIDs: ids,
	})
}

// DueForExpiry returns the ids of unpaid orders whose payment deadline has passed.
func (r *OrderRepository) DueForExpiry(ctx context.Context, now time.Time, limit int) ([]uint64, error) {
	var ids []uint64
	err := r.DB.WithContext(ctx).Model(&model.Order{}).
		Where("status = ? AND expires_at IS NOT NULL AND expires_at < ?", model.StatusAwaitingPayment, now).
		Order("id ASC").Limit(limit).Pluck("id", &ids).Error
	return ids, err
}

// DueForAutoDelivery returns shipped orders older than the given age.
func (r *OrderRepository) DueForAutoDelivery(ctx context.Context, olderThan time.Time, limit int) ([]uint64, error) {
	var ids []uint64
	err := r.DB.WithContext(ctx).Model(&model.Order{}).
		Where("status = ? AND shipped_at IS NOT NULL AND shipped_at < ?", model.StatusShipped, olderThan).
		Order("id ASC").Limit(limit).Pluck("id", &ids).Error
	return ids, err
}

// OrdersInCheckout returns a checkout's orders (without items) for event handlers.
func (r *OrderRepository) OrdersInCheckout(ctx context.Context, checkoutID string) ([]*model.Order, error) {
	var orders []*model.Order
	err := r.DB.WithContext(ctx).Where("checkout_id = ?", checkoutID).Order("id ASC").Find(&orders).Error
	return orders, err
}

// RequestRefund emits a checkout-level refund request (used when a payment arrived for orders that can no longer
// be paid); it is not attached to any single order.
func (r *OrderRepository) RequestRefund(ctx context.Context, checkoutID string, amountMinor int64, reason, key string) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return emitRefund(tx, checkoutID, 0, amountMinor, reason, key) })
}

// MarkPaymentRefunded records that the money of an order went back to the buyer (event from payment-service).
func (r *OrderRepository) MarkPaymentRefunded(ctx context.Context, orderID uint64) error {
	return r.DB.WithContext(ctx).Model(&model.Order{}).Where("id = ? AND payment_status = ?", orderID, model.PaymentPaid).
		Update("payment_status", model.PaymentRefunded).Error
}

// Backlog proxies the outbox backlog (for metrics).
func (r *OrderRepository) Backlog() (int64, float64) {
	n, age, _ := events.Backlog(r.DB)
	return n, age
}
