package model

import (
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Product struct {
	ID         uint64         `gorm:"primaryKey;autoIncrement"`
	Name       string         `gorm:"not null"`
	Price      float64        `gorm:"not null"`
	SellerID   uint64         `gorm:"not null"`
	Inventory  int64          `gorm:"not null"`
	Attributes datatypes.JSON `gorm:"not null"`
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}
