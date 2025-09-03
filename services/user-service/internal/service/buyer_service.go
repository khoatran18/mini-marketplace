package service

import (
	"context"
	"user-service/internal/service/adapter"
	"user-service/pkg/dto"
)

func (s *UserService) CreateBuyer(ctx context.Context, input *dto.CreateBuyerInput) (*dto.CreateBuyerOutput, error) {
	buyer, err := adapter.BuyerDTOToModel(input.Buyer)
	if err != nil {
		return nil, err
	}

	if err := s.UserRepo.CreateBuyer(ctx, buyer); err != nil {
		return nil, err
	}
	return &dto.CreateBuyerOutput{
		Message: "Buyer created successfully",
		Success: true,
	}, nil
}

func (s *UserService) GetBuyerByUserID(ctx context.Context, input *dto.GetBuyerByUserIDInput) (*dto.GetBuyerByUserIDOutput, error) {
	buyer, err := s.UserRepo.GetBuyerByUserID(ctx, input.UserID)
	if err != nil {
		return nil, err
	}
	buyerDTO, err := adapter.BuyerModelToDTO(buyer)
	if err != nil {
		return nil, err
	}
	return &dto.GetBuyerByUserIDOutput{
		Message: "Get buyer by UserID successfully",
		Success: true,
		Buyer:   buyerDTO,
	}, nil
}

func (s *UserService) UpdateBuyerByUserID(ctx context.Context, input *dto.UpdateBuyerByUserIDInput) (*dto.UpdateBuyerByUserIDOutput, error) {
	buyer, err := adapter.BuyerDTOToModel(input.Buyer)
	if err != nil {
		return nil, err
	}
	if err := s.UserRepo.UpdateBuyerByUserID(ctx, buyer); err != nil {
		return nil, err
	}
	return &dto.UpdateBuyerByUserIDOutput{
		Message: "Update buyer successfully",
		Success: true,
	}, nil
}

func (s *UserService) DelBuyerByUserID(ctx context.Context, input *dto.DelBuyerByUserIDInput) (*dto.DelBuyerByUserIDOutput, error) {
	if err := s.UserRepo.DelBuyerByUserID(ctx, input.UserID); err != nil {
		return nil, err
	}
	return &dto.DelBuyerByUserIDOutput{
		Message: "Delete Buyer successfully",
		Success: true,
	}, nil
}
