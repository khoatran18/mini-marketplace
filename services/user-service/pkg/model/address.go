package model

import "time"

// Address is a delivery address of a buyer. Orders copy it (snapshot) at checkout, so later edits or
// deletes never change existing orders.
type Address struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	UserID       uint64 `gorm:"not null;index"`
	Label        string `gorm:"not null;default:''"`
	ReceiverName string `gorm:"not null"`
	Phone        string `gorm:"not null"`
	Line1        string `gorm:"not null"`
	Ward         string `gorm:"not null;default:''"`
	District     string `gorm:"not null;default:''"`
	City         string `gorm:"not null"`
	IsDefault    bool   `gorm:"not null;default:false"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// MaxAddressesPerUser limits how many addresses one user can store.
const MaxAddressesPerUser = 10
