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

func (s *UserService) GetSellerByID(ctx context.Context, input *dto.GetSellerByIDInput) (*dto.GetSellerByIDOutput, error) {
	buyer, err := s.UserRepo.GetSellerByID(ctx, input.UserID)
	if err != nil {
		return nil, err
	}
	buyerDTO, err := adapter.SellerModelToDTO(buyer)
	if err != nil {
		return nil, err
	}
	return &dto.GetSellerByIDOutput{
		Message: "Get seller by ID successfully",
		Success: true,
		Seller:  buyerDTO,
	}, nil
}

func (s *UserService) UpdateSellerByID(ctx context.Context, input *dto.UpdateSellerByIDInput) (*dto.UpdateSellerByIDOutput, error) {
	// Check if seller existed
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
	if accountOutput.Role != "seller_admin" {
		return nil, errors.New("this user_id is not seller_admin")
	}

	// Update
	if err := s.UserRepo.UpdateSellerByID(ctx, buyer); err != nil {
		return nil, err
	}
	return &dto.UpdateSellerByIDOutput{
		Message: "Update Seller successfully",
		Success: true,
	}, nil
}

func (s *UserService) DelSellerByUserID(ctx context.Context, input *dto.DelSellerByIDInput) (*dto.DelSellerByIDOutput, error) {
	if err := s.UserRepo.DelSellerByID(ctx, input.UserID); err != nil {
		return nil, err
	}
	return &dto.DelSellerByIDOutput{
		Message: "Delete Seller successfully",
		Success: true,
	}, nil
}
