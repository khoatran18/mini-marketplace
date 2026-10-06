package model

import (
	"time"
)

// Order statuses. Allowed transitions:
//
//	PENDING -> SUCCESS   (inventory reserved by product-service)
//	PENDING -> FAILED    (inventory validation failed)
//	SUCCESS -> CANCELED  (buyer cancels, inventory is released)
//
// FAILED and CANCELED are terminal. A PENDING order is still being processed and cannot be canceled.
const (
	StatusPending  = "PENDING"
	StatusSuccess  = "SUCCESS"
	StatusFailed   = "FAILED"
	StatusCanceled = "CANCELED"
)

type Order struct {
	ID         uint64       `gorm:"primaryKey;AutoIncrement"`
	BuyerID    uint64       `gorm:"not null;index:order_index"`
	Status     string       `gorm:"not null;default:'PENDING';index:order_index"`
	TotalPrice float64      `gorm:"type:numeric(18,2);not null;default:0"`
	OrderItems []*OrderItem `gorm:"foreignKey:OrderID"` // 1 to many (in SQL, references often in child table)
	CreatedAt  time.Time    `gorm:"autoCreateTime"`
	UpdatedAt  time.Time    `gorm:"autoUpdateTime"`
}

type OrderItem struct {
	ID        uint64    `gorm:"primaryKey;AutoIncrement"`
	OrderID   uint64    `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;;index:order_item_index"`
	ProductID uint64    `gorm:"not null"`
	Quantity  int64     `gorm:"not null"`
	Price     float64   `gorm:"type:numeric(18,2);not null"`                      // unit price snapshot taken from product-service at order time
	Status    string    `gorm:"not null;default:'ACTIVE';index:order_item_index"` // ACTIVE, CANCELED
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}
