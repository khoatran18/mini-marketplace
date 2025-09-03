package repository

import (
	"context"
	"fmt"
	"user-service/pkg/model"
)

func (r *UserRepository) CreateSeller(ctx context.Context, seller *model.Seller) error {
	return r.DB.WithContext(ctx).Create(seller).Error
}

func (r *UserRepository) GetSellerByUserID(ctx context.Context, userID uint64) (*model.Seller, error) {
	var seller model.Seller
	if err := r.DB.WithContext(ctx).Where("user_id = ?", userID).First(&seller).Error; err != nil {
		return nil, err
	}
	return &seller, nil
}

func (r *UserRepository) UpdateSellerByUserID(ctx context.Context, seller *model.Seller) error {
	result := r.DB.WithContext(ctx).Where("user_id = ?", seller.UserID).Updates(seller)
	switch {
	case result.Error != nil:
		return result.Error
	case result.RowsAffected == 0:
		return fmt.Errorf("seller with user_id %d not found", seller.UserID)
	}
	return nil
}

func (r *UserRepository) DelSellerByUserID(ctx context.Context, userID uint64) error {
	result := r.DB.WithContext(ctx).Where("user_id = ?", userID).Delete(&model.Seller{})
	switch {
	case result.Error != nil:
		return result.Error
	case result.RowsAffected == 0:
		return fmt.Errorf("seller with user_id %d not found", userID)
	}
	return nil
}
