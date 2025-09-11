package service

import (
	"context"
	"errors"
	"user-service/internal/client/authclient"
	"user-service/internal/service/adapter"
	"user-service/pkg/dto"
)

func (s *UserService) CreateSeller(ctx context.Context, input *dto.CreateSellerInput) (*dto.CreateSellerOutput, error) {
	buyer, err := adapter.SellerDTOToModel(input.Seller)
	if err != nil {
		return nil, err
	}

	// Validate role and have store
	accountOutput, err := s.SCM.AuthServiceClient.GetStoreIDRoleByID(ctx, &authclient.GetStoreIDRoleByIDInput{
		ID: input.UserID,
	})
	if err != nil {
		return nil, err
	}
	if accountOutput.Role != "seller_admin" || accountOutput.StoreID != 0 {
		return nil, errors.New("this user_id is not seller_admin or already have seller")
	}

	// Create seller
	if err := s.UserRepo.CreateSeller(ctx, buyer, input.UserID); err != nil {
		return nil, err
	}
	return &dto.CreateSellerOutput{
		Message: "Seller created successfully",
		Success: true,
	}, nil
}

func (s *UserService) GetSellerByUserID(ctx context.Context, input *dto.GetSellerByUserIDInput) (*dto.GetSellerByUserIDOutput, error) {
	buyer, err := s.UserRepo.GetSellerByUserID(ctx, input.UserID)
	if err != nil {
		return nil, err
	}
	buyerDTO, err := adapter.SellerModelToDTO(buyer)
	if err != nil {
		return nil, err
	}
	return &dto.GetSellerByUserIDOutput{
		Message: "Get buyer by UserID successfully",
		Success: true,
		Seller:  buyerDTO,
	}, nil
}

func (s *UserService) UpdateSellerByUserID(ctx context.Context, input *dto.UpdateSellerByUserIDInput) (*dto.UpdateSellerByUserIDOutput, error) {
	buyer, err := adapter.SellerDTOToModel(input.Seller)
	if err != nil {
		return nil, err
	}
	if err := s.UserRepo.UpdateSellerByUserID(ctx, buyer); err != nil {
		return nil, err
	}
	return &dto.UpdateSellerByUserIDOutput{
		Message: "Update buyer successfully",
		Success: true,
	}, nil
}

func (s *UserService) DelSellerByUserID(ctx context.Context, input *dto.DelSellerByUserIDInput) (*dto.DelSellerByUserIDOutput, error) {
	if err := s.UserRepo.DelSellerByUserID(ctx, input.UserID); err != nil {
		return nil, err
	}
	return &dto.DelSellerByUserIDOutput{
		Message: "Delete Buyer successfully",
		Success: true,
	}, nil
}
