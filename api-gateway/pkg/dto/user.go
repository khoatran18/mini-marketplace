package dto

import "time"

type Buyer struct {
	UserID      uint64    `json:"user_id"`
	Name        string    `json:"name"`
	Gender      string    `json:"gender"`
	DateOfBirth time.Time `json:"date_of_birth"`
	Phone       string    `json:"phone"`
	Address     string    `json:"address"`
}
type Seller struct {
	ID          uint64    `json:"id"`
	Name        string    `json:"name"`
	BankAccount string    `json:"bank_account"`
	TaxCode     string    `json:"tax_code"`
	Description string    `json:"description"`
	DateOfBirth time.Time `json:"date_of_birth"`
	Phone       string    `json:"phone"`
	Address     string    `json:"address"`
}

// For Buyer

type CreateBuyerInput struct {
	Buyer *Buyer `json:"buyer"`
}
type CreateBuyerOutput struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

type UpdBuyByUseIDInput struct {
	Buyer *Buyer `json:"buyer"`
}
type UpdBuyByUseIDOutput struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

type DelBuyByUseIDInput struct {
	UserID uint64 `json:"user_id"`
}
type DelBuyByUseIDOutput struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

type GetBuyByUseIDInput struct {
	UserID uint64 `json:"user_id"`
}
type GetBuyByUseIDOutput struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
	Buyer   *Buyer `json:"buyer"`
}

// For Seller

type CreateSellerInput struct {
	Seller *Seller `json:"seller"`
	UserID uint64  `json:"user_id"`
}
type CreateSellerOutput struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

type UpdSelByIDInput struct {
	UserID uint64  `json:"user_id"`
	Seller *Seller `json:"seller"`
}
type UpdSelByIDOutput struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

type GetSelByIDInput struct {
	UserID uint64 `json:"user_id"`
}
type GetSelByIDOutput struct {
	Message string  `json:"message"`
	Success bool    `json:"success"`
	Seller  *Seller `json:"seller"`
}

type DelSelByIDInput struct {
	UserID uint64 `json:"user_id"`
}
type DelSelByIDOutput struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}
