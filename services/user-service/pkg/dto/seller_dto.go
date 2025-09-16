package dto

import (
	"time"
)

type Seller struct {
	ID          uint64
	Name        string
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
	UserID uint64
}
type CreateSellerOutput struct {
	Message string
	Success bool
}

// UpdateSellerByID

type UpdateSellerByIDInput struct {
	UserID uint64
	Seller *Seller
}
type UpdateSellerByIDOutput struct {
	Message string
	Success bool
}

// GetSellerByID

type GetSellerByIDInput struct {
	UserID uint64
}
type GetSellerByIDOutput struct {
	Seller  *Seller
	Message string
	Success bool
}

type DelSellerByIDInput struct {
	UserID uint64
}
type DelSellerByIDOutput struct {
	Message string
	Success bool
}
