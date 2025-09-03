package dto

import (
	"time"
)

type Seller struct {
	UserID      uint64
	Name        string
	Gender      string
	BankAccount string
	TaxCode     string
	Description string
	DateOfBirth time.Time
	Phone       string
	Address     string
}

// CreateSeller

type CreateSellerInput struct {
	Seller *Seller
}
type CreateSellerOutput struct {
	Message string
	Success bool
}

// UpdateSellerByUserID

type UpdateSellerByUserIDInput struct {
	Seller *Seller
}
type UpdateSellerByUserIDOutput struct {
	Message string
	Success bool
}

// GetSellerByUserID

type GetSellerByUserIDInput struct {
	UserID uint64
}
type GetSellerByUserIDOutput struct {
	Seller  *Seller
	Message string
	Success bool
}

type DelSellerByUserIDInput struct {
	UserID uint64
}
type DelSellerByUserIDOutput struct {
	Message string
	Success bool
}
