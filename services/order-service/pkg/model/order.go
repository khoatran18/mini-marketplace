package model

import (
	"time"
)

type Order struct {
	ID         uint64       `gorm:"primaryKey;AutoIncrement"`
	BuyerID    uint64       `gorm:"not null;index:order_index"`
	Status     string       `gorm:"not null;default:'PENDING';index:order_index"` // PENDING, ACTIVE (valid inventory), CANCELED, COMPLETED
	TotalPrice float64      `gorm:"not null;default:0"`
	OrderItems []*OrderItem `gorm:"foreignKey:OrderID"` // 1 to many (in SQL, references often in child table)
	CreatedAt  time.Time    `gorm:"autoCreateTime"`
	UpdatedAt  time.Time    `gorm:"autoUpdateTime"`
}

type OrderItem struct {
	ID        uint64    `gorm:"primaryKey;AutoIncrement"`
	OrderID   uint64    `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;;index:order_item_index"`
	ProductID uint64    `gorm:"not null"`
	Quantity  int64     `gorm:"not null"`
	Price     float64   `gorm:"not null"`
	Status    string    `gorm:"not null;default:'ACTIVE';index:order_item_index"` // ACTIVE, CANCELED
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}
