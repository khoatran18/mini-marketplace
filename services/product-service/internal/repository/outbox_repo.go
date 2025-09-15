package repository

import (
	"context"
	"errors"
	"product-service/pkg/outbox"

	"gorm.io/gorm"
)

func (r *ProductRepository) CreateOrUpdateValOrdEvent(tx *gorm.DB, orderID uint64, success bool, processed bool) error {
	if err := tx.Where("order_id = ?", orderID).First(&outbox.ValidateOrderEvent{}).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			pwdVersionEvent := &outbox.ValidateOrderEvent{
				OrderID:   orderID,
				Success:   success,
				Status:    "PENDING",
				Processed: processed,
			}
			if err = tx.Create(pwdVersionEvent).Error; err != nil {
				return err
			}
		}
		return err
	}

	return tx.Model(&outbox.ValidateOrderEvent{}).Where("order_id = ? AND processed = ?", orderID, true).Updates(map[string]interface{}{
		"success": success,
		"status":  "PENDING",
	}).Error
}

func (r *ProductRepository) GetProcessedValOrdEvent(ctx context.Context, orderID uint64) (bool, error) {
	var processed bool
	result := r.DB.WithContext(ctx).Model(&outbox.ValidateOrderEvent{}).Select("processed").Where("order_id = ?", orderID).Find(&processed)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	return processed, nil
}

func (r *ProductRepository) GetValOrdEventNotPublish(limit int) ([]*outbox.ValidateOrderKafkaEvent, error) {
	var pwdVersionEvents []*outbox.ValidateOrderKafkaEvent
	result := r.DB.Model(&outbox.ValidateOrderEvent{}).Where("status IN ? AND processed = ?", []string{"PENDING", "FAILED"}, true).Find(&pwdVersionEvents).Limit(limit)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return pwdVersionEvents, nil
}

func (r *ProductRepository) UpdateValOrdEventStatus(ctx context.Context, userID uint64, status string) error {
	return r.DB.WithContext(ctx).Model(&outbox.ValidateOrderEvent{}).Where("order_id = ?", userID).
		Updates(map[string]interface{}{"status": status}).Error
}
