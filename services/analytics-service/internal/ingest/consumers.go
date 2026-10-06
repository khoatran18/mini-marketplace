// Package ingest turns Kafka domain events and browser tracking events into ClickHouse rows.
package ingest

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"github.com/segmentio/kafka-go"

	"analytics-service/internal/store"
)

// Topics consumed (all produced through the services' transactional outbox).
const (
	TopicOrderStatus      = "order.status_changed"
	TopicPaymentSucceeded = "payment.succeeded"
	TopicPaymentFailed    = "payment.failed"
	TopicPaymentRefunded  = "payment.refunded"
	TopicProductChanged   = "product.changed"
	TopicInventoryChanged = "inventory.changed"
)

// Adder receives rows (implemented by *store.Sink).
type Adder interface{ Add(table string, row any) }

// Consumers holds the Kafka handlers. Every handler is idempotent: ClickHouse tables are ReplacingMergeTree keyed
// on the natural ids, so a redelivered message just writes the same row again.
type Consumers struct{ Out Adder }

// HandleOrderStatus consumes order.status_changed.
func (c *Consumers) HandleOrderStatus(_ context.Context, m *kafka.Message) error {
	var e struct {
		OrderID          uint64    `json:"order_id"`
		CheckoutID       string    `json:"checkout_id"`
		BuyerID          uint64    `json:"buyer_id"`
		StoreID          uint64    `json:"store_id"`
		Status           string    `json:"status"`
		PaymentMethod    string    `json:"payment_method"`
		PaymentStatus    string    `json:"payment_status"`
		SubtotalMinor    int64     `json:"subtotal_minor"`
		ShippingFeeMinor int64     `json:"shipping_fee_minor"`
		TotalMinor       int64     `json:"total_minor"`
		At               time.Time `json:"at"`
		Items            []struct {
			ItemID         uint64 `json:"item_id"`
			ProductID      uint64 `json:"product_id"`
			CategoryID     uint64 `json:"category_id"`
			Quantity       int64  `json:"quantity"`
			UnitPriceMinor int64  `json:"unit_price_minor"`
		} `json:"items"`
	}
	if err := json.Unmarshal(m.Value, &e); err != nil || e.OrderID == 0 || e.Status == "" {
		return nil // malformed: skip (retrying cannot fix it)
	}
	at := store.Time(e.At)
	c.Out.Add("fact_order_status", store.OrderStatusRow{
		OrderID: e.OrderID, Status: e.Status, CheckoutID: e.CheckoutID, BuyerID: e.BuyerID, StoreID: e.StoreID,
		PaymentMethod: e.PaymentMethod, PaymentStatus: e.PaymentStatus, SubtotalMinor: e.SubtotalMinor,
		ShippingFeeMinor: e.ShippingFeeMinor, TotalMinor: e.TotalMinor, At: at,
	})
	for _, it := range e.Items {
		c.Out.Add("fact_order_items", store.OrderItemRow{
			OrderID: e.OrderID, ItemID: it.ItemID, StoreID: e.StoreID, ProductID: it.ProductID, CategoryID: it.CategoryID,
			Qty: uint32(it.Quantity), UnitPriceMinor: it.UnitPriceMinor, At: at,
		})
	}
	return nil
}

// HandlePaymentSucceeded consumes payment.succeeded.
func (c *Consumers) HandlePaymentSucceeded(_ context.Context, m *kafka.Message) error {
	var e struct {
		PaymentID   uint64    `json:"payment_id"`
		CheckoutID  string    `json:"checkout_id"`
		BuyerID     uint64    `json:"buyer_id"`
		Method      string    `json:"method"`
		AmountMinor int64     `json:"amount_minor"`
		At          time.Time `json:"at"`
	}
	if err := json.Unmarshal(m.Value, &e); err != nil || e.PaymentID == 0 {
		return nil
	}
	c.Out.Add("fact_payment_events", store.PaymentEventRow{PaymentID: e.PaymentID, Kind: "succeeded", CheckoutID: e.CheckoutID,
		BuyerID: e.BuyerID, Method: e.Method, AmountMinor: e.AmountMinor, At: store.Time(e.At)})
	return nil
}

// HandlePaymentFailed consumes payment.failed.
func (c *Consumers) HandlePaymentFailed(_ context.Context, m *kafka.Message) error {
	var e struct {
		PaymentID   uint64    `json:"payment_id"`
		CheckoutID  string    `json:"checkout_id"`
		BuyerID     uint64    `json:"buyer_id"`
		Method      string    `json:"method"`
		FailureCode string    `json:"failure_code"`
		Terminal    bool      `json:"terminal"`
		At          time.Time `json:"at"`
	}
	if err := json.Unmarshal(m.Value, &e); err != nil || e.PaymentID == 0 {
		return nil
	}
	var term uint8
	if e.Terminal {
		term = 1
	}
	c.Out.Add("fact_payment_events", store.PaymentEventRow{PaymentID: e.PaymentID, Kind: "failed", CheckoutID: e.CheckoutID,
		BuyerID: e.BuyerID, Method: e.Method, FailureCode: e.FailureCode, Terminal: term, At: store.Time(e.At)})
	return nil
}

// HandlePaymentRefunded consumes payment.refunded.
func (c *Consumers) HandlePaymentRefunded(_ context.Context, m *kafka.Message) error {
	var e struct {
		PaymentID   uint64    `json:"payment_id"`
		RefundID    uint64    `json:"refund_id"`
		CheckoutID  string    `json:"checkout_id"`
		OrderID     uint64    `json:"order_id"`
		AmountMinor int64     `json:"amount_minor"`
		Reason      string    `json:"reason"`
		At          time.Time `json:"at"`
	}
	if err := json.Unmarshal(m.Value, &e); err != nil || e.RefundID == 0 {
		return nil
	}
	c.Out.Add("fact_refunds", store.RefundRow{RefundID: e.RefundID, PaymentID: e.PaymentID, CheckoutID: e.CheckoutID,
		OrderID: e.OrderID, AmountMinor: e.AmountMinor, Reason: e.Reason, At: store.Time(e.At)})
	return nil
}

// HandleProductChanged consumes product.changed.
func (c *Consumers) HandleProductChanged(_ context.Context, m *kafka.Message) error {
	var e struct {
		ProductID  uint64    `json:"product_id"`
		Op         string    `json:"op"`
		StoreID    uint64    `json:"store_id"`
		Name       string    `json:"name"`
		SKU        string    `json:"sku"`
		CategoryID uint64    `json:"category_id"`
		Brand      string    `json:"brand"`
		Price      float64   `json:"price"`
		Status     string    `json:"status"`
		UpdatedAt  time.Time `json:"updated_at"`
	}
	if err := json.Unmarshal(m.Value, &e); err != nil || e.ProductID == 0 {
		return nil
	}
	if e.Op == "delete" {
		e.Status = "deleted"
	}
	c.Out.Add("dim_products", store.ProductRow{ProductID: e.ProductID, StoreID: e.StoreID, Name: e.Name, SKU: e.SKU,
		CategoryID: e.CategoryID, Brand: e.Brand, PriceMinor: int64(math.Round(e.Price * 100)), Status: e.Status,
		UpdatedAt: store.Time(e.UpdatedAt)})
	return nil
}

// HandleInventoryChanged consumes inventory.changed.
func (c *Consumers) HandleInventoryChanged(_ context.Context, m *kafka.Message) error {
	var e struct {
		ProductID uint64    `json:"product_id"`
		StoreID   uint64    `json:"store_id"`
		Delta     int64     `json:"delta"`
		Available int64     `json:"available"`
		Reserved  int64     `json:"reserved"`
		Level     string    `json:"level"`
		Reason    string    `json:"reason"`
		RefType   string    `json:"ref_type"`
		RefID     string    `json:"ref_id"`
		At        time.Time `json:"at"`
	}
	if err := json.Unmarshal(m.Value, &e); err != nil || e.ProductID == 0 {
		return nil
	}
	c.Out.Add("fact_inventory", store.InventoryRow{ProductID: e.ProductID, StoreID: e.StoreID, Delta: e.Delta,
		Available: e.Available, Reserved: e.Reserved, Level: e.Level, Reason: e.Reason, RefType: e.RefType,
		RefID: e.RefID, At: store.Time(e.At)})
	return nil
}

// Topics maps every consumed topic to its handler (used by main and the tests).
func (c *Consumers) Topics() map[string]func(context.Context, *kafka.Message) error {
	return map[string]func(context.Context, *kafka.Message) error{
		TopicOrderStatus:      c.HandleOrderStatus,
		TopicPaymentSucceeded: c.HandlePaymentSucceeded,
		TopicPaymentFailed:    c.HandlePaymentFailed,
		TopicPaymentRefunded:  c.HandlePaymentRefunded,
		TopicProductChanged:   c.HandleProductChanged,
		TopicInventoryChanged: c.HandleInventoryChanged,
	}
}
