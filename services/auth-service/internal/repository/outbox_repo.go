package repository

import (
	"auth-service/pkg/outbox"
	"context"
	"errors"

	"gorm.io/gorm"
)

func (r *AccountRepository) CreateOrUpdatePwdVersionEvent(tx *gorm.DB, userID uint64, pwdVersion int64) error {
	if err := tx.Where("user_id = ?", userID).First(&outbox.PwdVersionEvent{}).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			pwdVersionEvent := &outbox.PwdVersionEvent{
				UserID:     userID,
				PwdVersion: pwdVersion,
				Status:     "PENDING",
			}
			if err = tx.Create(pwdVersionEvent).Error; err != nil {
				return err
			}
		}
		return err
	}

	return tx.Model(&outbox.PwdVersionEvent{}).Where("user_id = ?", userID).Updates(map[string]interface{}{
		"pwd_version": pwdVersion,
		"status":      "PENDING",
	}).Error
}

func (r *AccountRepository) GetPwdVersionEventNotPublish(limit int) ([]*outbox.PwdVersionKafkaEvent, error) {
	var pwdVersionEvents []*outbox.PwdVersionKafkaEvent
	result := r.DB.Model(&outbox.PwdVersionEvent{}).Where("status IN ?", []string{"PENDING", "FAILED"}).Find(&pwdVersionEvents).Limit(limit)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return pwdVersionEvents, nil
}

func (r *AccountRepository) UpdatePwdVersionEventStatus(ctx context.Context, userID uint64, status string) error {
	return r.DB.WithContext(ctx).Model(&outbox.PwdVersionEvent{}).Where("user_id = ?", userID).
		Updates(map[string]interface{}{"status": status}).Error
}
