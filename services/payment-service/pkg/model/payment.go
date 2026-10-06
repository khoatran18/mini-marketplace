package model

import (
	"time"

	"gorm.io/datatypes"
)

// Payment statuses.
const (
	StatusRequiresAction  = "REQUIRES_ACTION" // waiting for the buyer (card details, OTP, wallet approval, transfer)
	StatusProcessing      = "PROCESSING"      // submitted, the (simulated) provider will answer through a webhook
	StatusSucceeded       = "SUCCEEDED"
	StatusFailed          = "FAILED" // too many failed attempts
	StatusCanceled        = "CANCELED"
	StatusExpired         = "EXPIRED"
	StatusPartialRefunded = "PARTIALLY_REFUNDED"
	StatusRefunded        = "REFUNDED"
	ProviderMockPay       = "mockpay"
	CurrencyVND           = "VND"
	MaxFailedAttempts     = 5
	AttemptPending        = "PENDING"
	AttemptSucceeded      = "SUCCEEDED"
	AttemptFailed         = "FAILED"
	RefundSucceeded       = "SUCCEEDED"
	RefundFailed          = "FAILED"
	WebhookPending        = "PENDING"
	WebhookDelivered      = "DELIVERED"
	WebhookGaveUp         = "GAVE_UP"
	EventSucceeded        = "payment.succeeded"
	EventFailed           = "payment.failed"
)

// Payment methods (same values as order-service).
const (
	MethodCard     = "MOCK_CARD"
	MethodWallet   = "MOCK_WALLET"
	MethodTransfer = "MOCK_BANK_TRANSFER"
)

// Payment is the (simulated) payment of one checkout. Amounts are integer minor units (1/100 VND, see ADR-7).
type Payment struct {
	ID            uint64         `gorm:"primaryKey;autoIncrement"`
	CheckoutID    string         `gorm:"not null;uniqueIndex"`
	BuyerID       uint64         `gorm:"not null;index"`
	Method        string         `gorm:"not null"`
	Status        string         `gorm:"not null;index"`
	AmountMinor   int64          `gorm:"not null"`
	Currency      string         `gorm:"not null;default:'VND'"`
	Provider      string         `gorm:"not null;default:'mockpay'"`
	ProviderRef   string         `gorm:"not null;default:''"`
	FailureCode   string         `gorm:"not null;default:''"`
	CardLast4     string         `gorm:"not null;default:''"`
	FailedCount   int            `gorm:"not null;default:0"`
	ThreeDSPassed bool           `gorm:"not null;default:false"` // the 3-D Secure challenge was shown (next confirm needs the OTP)
	OrderIDs      datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	ExpiresAt     time.Time      `gorm:"not null;index"`
	PaidAt        *time.Time
	RefundedMinor int64 `gorm:"not null;default:0"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Attempt is one try of the buyer (history for support and analytics). No card data is stored.
type Attempt struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	PaymentID   uint64    `gorm:"not null;index"`
	Scenario    string    `gorm:"not null"`
	Status      string    `gorm:"not null"`
	FailureCode string    `gorm:"not null;default:''"`
	At          time.Time `gorm:"not null;autoCreateTime"`
}

// Refund is money returned to the buyer. RefundKey makes requests idempotent.
type Refund struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	PaymentID   uint64    `gorm:"not null;index"`
	OrderID     uint64    `gorm:"not null;default:0"`
	RefundKey   string    `gorm:"not null;uniqueIndex"`
	AmountMinor int64     `gorm:"not null"`
	Reason      string    `gorm:"not null;default:''"`
	Status      string    `gorm:"not null"`
	At          time.Time `gorm:"not null;autoCreateTime"`
}

// WebhookDelivery is a simulated provider notification waiting to be sent to our own webhook endpoint.
type WebhookDelivery struct {
	ID              uint64         `gorm:"primaryKey;autoIncrement"`
	PaymentID       uint64         `gorm:"not null;index"`
	ProviderEventID string         `gorm:"not null"`
	Type            string         `gorm:"not null"`
	Payload         datatypes.JSON `gorm:"not null"`
	DeliverAt       time.Time      `gorm:"not null;index:idx_webhook_due,priority:2"`
	Attempts        int            `gorm:"not null;default:0"`
	Status          string         `gorm:"not null;default:'PENDING';index:idx_webhook_due,priority:1"`
	LastCode        int            `gorm:"not null;default:0"`
	DeliveredAt     *time.Time
	CreatedAt       time.Time
}

// ProcessedEvent records provider events already applied (webhooks are at-least-once and may repeat).
type ProcessedEvent struct {
	ProviderEventID string    `gorm:"primaryKey"`
	At              time.Time `gorm:"autoCreateTime"`
}

// ValidMethod reports whether method is one of the simulated methods.
func ValidMethod(m string) bool { return m == MethodCard || m == MethodWallet || m == MethodTransfer }
