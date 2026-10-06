package repository

import (
	"order-service/pkg/events"
	"order-service/pkg/model"
	"strconv"
	"time"

	"gorm.io/gorm"
)

// Topics published by order-service through the transactional outbox (pkg/events).
const (
	TopicCreateOrder     = "order.create_order"     // ask product-service to reserve stock
	TopicCancelOrder     = "order.cancel_order"     // ask product-service to release the reserved stock
	TopicStatusChanged   = "order.status_changed"   // every status change (analytics, product-service ship)
	TopicRefundRequested = "order.refund_requested" // ask payment-service to refund
	TopicPaymentRequest  = "payment.requested"      // ask payment-service to create the payment of a checkout
)

// Topics consumed.
const (
	TopicValidateOrder   = "product.validate_order"
	TopicPaymentSucceed  = "payment.succeeded"
	TopicPaymentRefunded = "payment.refunded"
)

// Money in events is an integer count of minor units (1/100 of the currency unit), see ADR-7.
func minor(amount float64) int64 { return int64(amount*100 + 0.5*sign(amount)) }

func sign(f float64) float64 {
	if f < 0 {
		return -1
	}
	return 1
}

// ItemEvent is an order line inside events.
type ItemEvent struct {
	ItemID         uint64 `json:"item_id"`
	ProductID      uint64 `json:"product_id"`
	CategoryID     uint64 `json:"category_id"`
	Quantity       int64  `json:"quantity"`
	UnitPriceMinor int64  `json:"unit_price_minor"`
}

// StatusChangedEvent is the payload of order.status_changed.
type StatusChangedEvent struct {
	V                int         `json:"v"`
	OrderID          uint64      `json:"order_id"`
	CheckoutID       string      `json:"checkout_id"`
	BuyerID          uint64      `json:"buyer_id"`
	StoreID          uint64      `json:"store_id"`
	Status           string      `json:"status"`
	Prev             string      `json:"prev"`
	ActorType        string      `json:"actor_type"`
	PaymentMethod    string      `json:"payment_method"`
	PaymentStatus    string      `json:"payment_status"`
	SubtotalMinor    int64       `json:"subtotal_minor"`
	ShippingFeeMinor int64       `json:"shipping_fee_minor"`
	TotalMinor       int64       `json:"total_minor"`
	At               time.Time   `json:"at"`
	Items            []ItemEvent `json:"items"`
}

// StockItem is a line of order.create_order / order.cancel_order (consumed by product-service).
type StockItem struct {
	ProductID uint64 `json:"product_id"`
	Quantity  int64  `json:"quantity"`
}

// StockEvent is the payload of order.create_order and order.cancel_order.
type StockEvent struct {
	OrderID uint64      `json:"order_id"`
	Items   []StockItem `json:"items"`
}

// RefundRequestedEvent asks payment-service to refund part of a checkout's payment. RefundKey makes the
// request idempotent (payment-service refunds each key once).
type RefundRequestedEvent struct {
	V           int    `json:"v"`
	CheckoutID  string `json:"checkout_id"`
	OrderID     uint64 `json:"order_id"`
	AmountMinor int64  `json:"amount_minor"`
	Reason      string `json:"reason"`
	RefundKey   string `json:"refund_key"`
}

// PaymentRequestedEvent asks payment-service to create the payment of a checkout once every order of it has been
// validated: the amount covers only the orders that are awaiting payment.
type PaymentRequestedEvent struct {
	V           int       `json:"v"`
	CheckoutID  string    `json:"checkout_id"`
	BuyerID     uint64    `json:"buyer_id"`
	Method      string    `json:"method"`
	AmountMinor int64     `json:"amount_minor"`
	ExpiresAt   time.Time `json:"expires_at"`
	OrderIDs    []uint64  `json:"order_ids"`
}

func itemEvents(items []*model.OrderItem) []ItemEvent {
	out := make([]ItemEvent, 0, len(items))
	for _, it := range items {
		out = append(out, ItemEvent{ItemID: it.ID, ProductID: it.ProductID, CategoryID: it.CategoryID, Quantity: it.Quantity, UnitPriceMinor: minor(it.Price)})
	}
	return out
}

func stockItems(items []*model.OrderItem) []StockItem {
	out := make([]StockItem, 0, len(items))
	for _, it := range items {
		out = append(out, StockItem{ProductID: it.ProductID, Quantity: it.Quantity})
	}
	return out
}

func orderKey(id uint64) string { return strconv.FormatUint(id, 10) }

func emitStatusChanged(tx *gorm.DB, o *model.Order, prev, actorType string) error {
	return events.Emit(tx, TopicStatusChanged, orderKey(o.ID), StatusChangedEvent{
		V: 1, OrderID: o.ID, CheckoutID: o.CheckoutID, BuyerID: o.BuyerID, StoreID: o.StoreID, Status: o.Status, Prev: prev,
		ActorType: actorType, PaymentMethod: o.PaymentMethod, PaymentStatus: o.PaymentStatus,
		SubtotalMinor: minor(o.Subtotal), ShippingFeeMinor: minor(o.ShippingFee), TotalMinor: minor(o.TotalPrice),
		At: time.Now().UTC(), Items: itemEvents(o.OrderItems),
	})
}

func emitCreateOrder(tx *gorm.DB, o *model.Order) error {
	return events.Emit(tx, TopicCreateOrder, orderKey(o.ID), StockEvent{OrderID: o.ID, Items: stockItems(o.OrderItems)})
}

func emitReleaseStock(tx *gorm.DB, o *model.Order) error {
	return events.Emit(tx, TopicCancelOrder, orderKey(o.ID), StockEvent{OrderID: o.ID, Items: stockItems(o.OrderItems)})
}

// emitRefund asks payment-service for a refund. orderID is 0 for refunds that belong to the checkout as a whole
// (money received for orders that could no longer be paid).
func emitRefund(tx *gorm.DB, checkoutID string, orderID uint64, amountMinor int64, reason, key string) error {
	return events.Emit(tx, TopicRefundRequested, checkoutID, RefundRequestedEvent{
		V: 1, CheckoutID: checkoutID, OrderID: orderID, AmountMinor: amountMinor, Reason: reason, RefundKey: key,
	})
}
