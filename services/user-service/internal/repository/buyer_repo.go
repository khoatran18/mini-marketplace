package repository

import (
	"context"
	"fmt"
	"user-service/pkg/model"
)

func (r *UserRepository) CreateBuyer(ctx context.Context, buyer *model.Buyer) error {
	return r.DB.WithContext(ctx).Create(buyer).Error
}

func (r *UserRepository) GetBuyerByUserID(ctx context.Context, user_id uint64) (*model.Buyer, error) {
	var buyer model.Buyer
	if err := r.DB.WithContext(ctx).Where("user_id = ?", user_id).First(&buyer).Error; err != nil {
		return nil, err
	}
	return &buyer, nil
}

func (r *UserRepository) UpdateBuyerByUserID(ctx context.Context, buyer *model.Buyer) error {
	result := r.DB.WithContext(ctx).Where("user_id = ?", buyer.UserID).Updates(buyer)
	switch {
	case result.Error != nil:
		return result.Error
	case result.RowsAffected == 0:
		return fmt.Errorf("seller with user_id %d not found", buyer.UserID)
	}
	return nil
}

func (r *UserRepository) DelBuyerByUserID(ctx context.Context, userID uint64) error {
	result := r.DB.WithContext(ctx).Where("user_id = ?", userID).Delete(&model.Buyer{})
	switch {
	case result.Error != nil:
		return result.Error
	case result.RowsAffected == 0:
		return fmt.Errorf("seller with user_id %d not found", userID)
	}
	return nil
}
