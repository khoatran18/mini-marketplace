package adapter

import (
	"user-service/pkg/dto"
	"user-service/pkg/model"
)

func BuyerDTOToModel(buyer *dto.Buyer) (*model.Buyer, error) {
	return &model.Buyer{
		UserID:      buyer.UserID,
		Name:        buyer.Name,
		Gender:      buyer.Gender,
		DateOfBirth: buyer.DateOfBirth,
		Phone:       buyer.Phone,
		Address:     buyer.Address,
	}, nil
}

func BuyerModelToDTO(buyer *model.Buyer) (*dto.Buyer, error) {
	return &dto.Buyer{
		UserID:      buyer.UserID,
		Name:        buyer.Name,
		Gender:      buyer.Gender,
		DateOfBirth: buyer.DateOfBirth,
		Phone:       buyer.Phone,
		Address:     buyer.Address,
	}, nil
}

func SellerDTOToModel(seller *dto.Seller) (*model.Seller, error) {
	return &model.Seller{
		Name:        seller.Name,
		BankAccount: seller.BankAccount,
		TaxCode:     seller.TaxCode,
		Description: seller.Description,
		DateOfBirth: seller.DateOfBirth,
		Phone:       seller.Phone,
		Address:     seller.Address,
	}, nil
}

func SellerModelToDTO(seller *model.Seller) (*dto.Seller, error) {
	return &dto.Seller{
		ID:          seller.ID,
		Name:        seller.Name,
		BankAccount: seller.BankAccount,
		TaxCode:     seller.TaxCode,
		Description: seller.Description,
		DateOfBirth: seller.DateOfBirth,
		Phone:       seller.Phone,
		Address:     seller.Address,
	}, nil
}
