package model

import (
	"time"

	"gorm.io/gorm"
)

type Seller struct {
	UserID      uint64 `gorm:"primary_key"`
	Name        string `gorm:"not null"`
	Gender      string `gorm:"not null"`
	BankAccount string `gorm:"not null"`
	TaxCode     string `gorm:"not null"`
	Description string `gorm:"not null"`
	DateOfBirth time.Time
	Phone       string         `gorm:"not null"`
	Address     string         `gorm:"not null"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}
