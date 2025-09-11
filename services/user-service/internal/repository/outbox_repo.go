package repository

import (
	"context"
	"user-service/pkg/outbox"

	"gorm.io/gorm"
)

func (r *UserRepository) CreateCreateSellerEvent(tx *gorm.DB, sellerID uint64, userID uint64) error {
	pwdVersionEvent := &outbox.CreateSellerEvent{
		SellerID: sellerID,
		UserID:   userID,
		Status:   "PENDING",
	}
	if err := tx.Create(pwdVersionEvent).Error; err != nil {
		return err
	}
	return nil
}

func (r *UserRepository) GetCreateSellerEventNotPublish(limit int) ([]*outbox.CreateSellerKafkaEvent, error) {
	var pwdVersionEvents []*outbox.CreateSellerKafkaEvent
	result := r.DB.Model(&outbox.CreateSellerEvent{}).Where("status IN ?", []string{"PENDING", "FAILED"}).Find(&pwdVersionEvents).Limit(limit)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return pwdVersionEvents, nil
}

func (r *UserRepository) UpdateCreateSellerEventStatus(ctx context.Context, sellerID uint64, status string) error {
	return r.DB.WithContext(ctx).Model(&outbox.CreateSellerEvent{}).Where("seller_id = ?", sellerID).
		Updates(map[string]interface{}{"status": status}).Error
}
