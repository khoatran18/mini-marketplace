package model

import (
	"time"

	"gorm.io/datatypes"
)

// Order statuses.
//
//	PENDING            created, product-service is reserving the stock
//	AWAITING_PAYMENT   stock reserved, waiting for an online payment (expires at ExpiresAt)
//	CONFIRMED          stock reserved, cash on delivery
//	PAID               online payment succeeded
//	SHIPPED            handed to the carrier (reserved goods left the warehouse)
//	DELIVERED          received (COD: cash collected, payment status PAID)
//	REFUND_REQUESTED   buyer asked to return a delivered order
//	REFUNDED           return approved and money refunded
//	CANCELED           canceled before shipping (buyer, seller or admin)
//	EXPIRED            not paid in time, stock released
//	FAILED             stock was not available
const (
	StatusPending         = "PENDING"
	StatusAwaitingPayment = "AWAITING_PAYMENT"
	StatusConfirmed       = "CONFIRMED"
	StatusPaid            = "PAID"
	StatusShipped         = "SHIPPED"
	StatusDelivered       = "DELIVERED"
	StatusRefundRequested = "REFUND_REQUESTED"
	StatusRefunded        = "REFUNDED"
	StatusCanceled        = "CANCELED"
	StatusExpired         = "EXPIRED"
	StatusFailed          = "FAILED"
	// LegacyStatusSuccess is the pre-payment name of "stock reserved"; Migrate renames it to CONFIRMED.
	LegacyStatusSuccess = "SUCCESS"
)

// AllStatuses lists every valid status.
var AllStatuses = []string{
	StatusPending, StatusAwaitingPayment, StatusConfirmed, StatusPaid, StatusShipped, StatusDelivered,
	StatusRefundRequested, StatusRefunded, StatusCanceled, StatusExpired, StatusFailed,
}

// Payment methods and statuses.
const (
	MethodCOD          = "COD"
	MethodMockCard     = "MOCK_CARD"
	MethodMockWallet   = "MOCK_WALLET"
	MethodMockTransfer = "MOCK_BANK_TRANSFER"

	PaymentUnpaid   = "UNPAID"
	PaymentPaid     = "PAID"
	PaymentRefunded = "REFUNDED"
)

// IsOnline reports whether the method needs a payment before the order can be fulfilled.
func IsOnline(method string) bool { return method != MethodCOD }

// ValidMethod reports whether method is supported.
func ValidMethod(method string) bool {
	switch method {
	case MethodCOD, MethodMockCard, MethodMockWallet, MethodMockTransfer:
		return true
	}
	return false
}

// Order is one store's part of a checkout. A checkout with items of several stores creates one order per
// store; all of them share CheckoutID and are paid together.
type Order struct {
	ID         uint64       `gorm:"primaryKey;AutoIncrement"`
	BuyerID    uint64       `gorm:"not null;index:order_index"`
	Status     string       `gorm:"not null;default:'PENDING';index:order_index"`
	TotalPrice float64      `gorm:"type:numeric(18,2);not null;default:0"`
	OrderItems []*OrderItem `gorm:"foreignKey:OrderID"`
	CreatedAt  time.Time    `gorm:"autoCreateTime"`
	UpdatedAt  time.Time    `gorm:"autoUpdateTime"`

	CheckoutID      string         `gorm:"not null;default:'';index"`
	StoreID         uint64         `gorm:"not null;default:0;index:idx_orders_store_status,priority:1"`
	PaymentMethod   string         `gorm:"not null;default:'COD'"`
	PaymentStatus   string         `gorm:"not null;default:'UNPAID'"`
	Subtotal        float64        `gorm:"type:numeric(18,2);not null;default:0"`
	ShippingFee     float64        `gorm:"type:numeric(18,2);not null;default:0"`
	ShippingAddress datatypes.JSON `gorm:"type:jsonb"`
	Note            string         `gorm:"not null;default:''"`
	ExpiresAt       *time.Time     `gorm:"index"`
	PaidAt          *time.Time
	ShippedAt       *time.Time
	DeliveredAt     *time.Time
	CanceledAt      *time.Time
	CancelReason    string `gorm:"not null;default:''"`
	CanceledBy      string `gorm:"not null;default:''"`
	Carrier         string `gorm:"not null;default:''"`
	TrackingCode    string `gorm:"not null;default:''"`
	ReturnReason    string `gorm:"not null;default:''"`
	ReturnRejected  bool   `gorm:"not null;default:false"`
}

// OrderItem is one product line with a snapshot of the catalog data at order time.
type OrderItem struct {
	ID        uint64    `gorm:"primaryKey;AutoIncrement"`
	OrderID   uint64    `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;;index:order_item_index"`
	ProductID uint64    `gorm:"not null"`
	Quantity  int64     `gorm:"not null"`
	Price     float64   `gorm:"type:numeric(18,2);not null"`                      // unit price snapshot taken from product-service at order time
	Status    string    `gorm:"not null;default:'ACTIVE';index:order_item_index"` // ACTIVE, CANCELED
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`

	ProductName string  `gorm:"not null;default:''"`
	SKU         string  `gorm:"not null;default:''"`
	ImageURL    string  `gorm:"not null;default:''"`
	StoreID     uint64  `gorm:"not null;default:0"`
	CategoryID  uint64  `gorm:"not null;default:0"`
	LineTotal   float64 `gorm:"type:numeric(18,2);not null;default:0"`
}

// OrderStatusHistory records every status change (timeline in the UI, audit, SLA analytics).
type OrderStatusHistory struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"`
	OrderID    uint64    `gorm:"not null;index"`
	FromStatus string    `gorm:"not null;default:''"`
	ToStatus   string    `gorm:"not null"`
	ActorType  string    `gorm:"not null"`
	ActorID    uint64    `gorm:"not null;default:0"`
	Reason     string    `gorm:"not null;default:''"`
	At         time.Time `gorm:"not null;autoCreateTime"`
}

// Checkout groups the orders created by one "place order" action and makes it idempotent.
type Checkout struct {
	ID             string    `gorm:"primaryKey"`
	BuyerID        uint64    `gorm:"not null;uniqueIndex:idx_checkout_idem,priority:1"`
	IdempotencyKey string    `gorm:"not null;uniqueIndex:idx_checkout_idem,priority:2"`
	PaymentMethod  string    `gorm:"not null"`
	Total          float64   `gorm:"type:numeric(18,2);not null"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
}

// CartItem is one line of a buyer's server-side cart.
type CartItem struct {
	UserID    uint64    `gorm:"primaryKey;autoIncrement:false"`
	ProductID uint64    `gorm:"primaryKey;autoIncrement:false"`
	Quantity  int64     `gorm:"not null"`
	AddedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// MaxCartLines bounds the number of different products in a cart.
const MaxCartLines = 50
