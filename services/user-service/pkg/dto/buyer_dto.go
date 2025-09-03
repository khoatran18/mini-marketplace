package dto

import (
	"time"
)

type Buyer struct {
	UserID      uint64
	Name        string
	Gender      string
	DateOfBirth time.Time
	Phone       string
	Address     string
}

// CreateBuyer

type CreateBuyerInput struct {
	Buyer *Buyer
}
type CreateBuyerOutput struct {
	Message string
	Success bool
}

// UpdateBuyerByUserID

type UpdateBuyerByUserIDInput struct {
	Buyer *Buyer
}
type UpdateBuyerByUserIDOutput struct {
	Message string
	Success bool
}

// GetBuyerByUserID

type GetBuyerByUserIDInput struct {
	UserID uint64
}
type GetBuyerByUserIDOutput struct {
	Buyer   *Buyer
	Message string
	Success bool
}

type DelBuyerByUserIDInput struct {
	UserID uint64
}
type DelBuyerByUserIDOutput struct {
	Message string
	Success bool
}
