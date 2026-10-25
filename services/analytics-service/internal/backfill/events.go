// Package backfill rebuilds the ClickHouse fact / dimension tables from Postgres (read-only) and reconciles the
// two stores. Rows are NOT built a second time here: Postgres rows are turned into the very same JSON payloads
// the services publish to Kafka and fed to ingest.Consumers, so a backfilled row is identical to a streamed one.
package backfill

import "time"

// Payloads of the Kafka events (mirror of order-service / payment-service / product-service producers).
// Only the fields analytics-service consumes are listed.

type itemEvent struct {
	ItemID         uint64 `json:"item_id"`
	ProductID      uint64 `json:"product_id"`
	CategoryID     uint64 `json:"category_id"`
	Quantity       int64  `json:"quantity"`
	UnitPriceMinor int64  `json:"unit_price_minor"`
}

type statusChangedEvent struct {
	V                int         `json:"v"`
	OrderID          uint64      `json:"order_id"`
	CheckoutID       string      `json:"checkout_id"`
	BuyerID          uint64      `json:"buyer_id"`
	StoreID          uint64      `json:"store_id"`
	Status           string      `json:"status"`
	PaymentMethod    string      `json:"payment_method"`
	PaymentStatus    string      `json:"payment_status"`
	SubtotalMinor    int64       `json:"subtotal_minor"`
	ShippingFeeMinor int64       `json:"shipping_fee_minor"`
	TotalMinor       int64       `json:"total_minor"`
	At               time.Time   `json:"at"`
	Items            []itemEvent `json:"items"`
}

type paymentSucceededEvent struct {
	V           int       `json:"v"`
	PaymentID   uint64    `json:"payment_id"`
	CheckoutID  string    `json:"checkout_id"`
	BuyerID     uint64    `json:"buyer_id"`
	Method      string    `json:"method"`
	AmountMinor int64     `json:"amount_minor"`
	At          time.Time `json:"at"`
}

type paymentFailedEvent struct {
	V           int       `json:"v"`
	PaymentID   uint64    `json:"payment_id"`
	CheckoutID  string    `json:"checkout_id"`
	BuyerID     uint64    `json:"buyer_id"`
	Method      string    `json:"method"`
	FailureCode string    `json:"failure_code"`
	Terminal    bool      `json:"terminal"`
	At          time.Time `json:"at"`
}

type paymentRefundedEvent struct {
	V           int       `json:"v"`
	PaymentID   uint64    `json:"payment_id"`
	RefundID    uint64    `json:"refund_id"`
	CheckoutID  string    `json:"checkout_id"`
	OrderID     uint64    `json:"order_id"`
	AmountMinor int64     `json:"amount_minor"`
	Reason      string    `json:"reason"`
	At          time.Time `json:"at"`
}

type productChangedEvent struct {
	V          int       `json:"v"`
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

type inventoryChangedEvent struct {
	V         int       `json:"v"`
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

// stockLevel mirrors product-service model.Product.StockLevel (default threshold 5).
func stockLevel(available, threshold int64) string {
	if threshold <= 0 {
		threshold = 5
	}
	switch {
	case available <= 0:
		return "none"
	case available <= threshold:
		return "low"
	default:
		return "ok"
	}
}

// maxFailedAttempts mirrors payment-service model.MaxFailedAttempts: the Nth failed attempt is terminal.
const maxFailedAttempts = 5
