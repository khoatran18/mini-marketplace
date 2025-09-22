package repository

import (
	"context"
	"fmt"
	"user-service/pkg/model"

	"gorm.io/gorm"
)

func (r *UserRepository) CreateSeller(ctx context.Context, seller *model.Seller, userID uint64) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {

		// Create seller
		if err := tx.Create(seller).Error; err != nil {
			return err
		}

		// Create event in outboxDB
		if err := r.CreateCreateSellerEvent(tx, seller.ID, userID); err != nil {
			return err
		}
		return nil
	})
}

func (r *UserRepository) GetSellerByID(ctx context.Context, id uint64) (*model.Seller, error) {
	var seller model.Seller
	if err := r.DB.WithContext(ctx).Where("id = ?", id).First(&seller).Error; err != nil {
		return nil, err
	}
	return &seller, nil
}

func (r *UserRepository) UpdateSellerByID(ctx context.Context, seller *model.Seller) error {
	result := r.DB.WithContext(ctx).Where("id = ?", seller.ID).Updates(seller)
	switch {
	case result.Error != nil:
		return result.Error
	case result.RowsAffected == 0:
		return fmt.Errorf("seller with ID %d not found", seller.ID)
	}
	return nil
}

func (r *UserRepository) DelSellerByID(ctx context.Context, id uint64) error {
	result := r.DB.WithContext(ctx).Where("id = ?", id).Delete(&model.Seller{})
	switch {
	case result.Error != nil:
		return result.Error
	case result.RowsAffected == 0:
		return fmt.Errorf("seller with user_id %d not found", id)
	}
	return nil
}
