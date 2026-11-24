package repository

import (
	"auth-service/pkg/outbox"
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

func (r *AccountRepository) CreateOrUpdatePwdVersionEvent(tx *gorm.DB, userID uint64, pwdVersion int64) error {
	if err := tx.Where("user_id = ?", userID).First(&outbox.PwdVersionEvent{}).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			pwdVersionEvent := &outbox.PwdVersionEvent{
				UserID:     userID,
				PwdVersion: pwdVersion,
				Status:     "PENDING",
				ChangedAt:  time.Now(),
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
		"changed_at":  time.Now(),
	}).Error
}

func (r *AccountRepository) GetPwdVersionEventNotPublish(limit int) ([]*outbox.PwdVersionKafkaEvent, error) {
	var pwdVersionEvents []*outbox.PwdVersionKafkaEvent
	err := r.DB.Model(&outbox.PwdVersionEvent{}).Where("status IN ?", []string{"PENDING", "FAILED"}).
		Order("created_at ASC").Limit(limit).Find(&pwdVersionEvents).Error
	return pwdVersionEvents, err
}

func (r *AccountRepository) UpdatePwdVersionEventStatus(ctx context.Context, userID uint64, status string) error {
	return r.DB.WithContext(ctx).Model(&outbox.PwdVersionEvent{}).Where("user_id = ?", userID).
		Updates(map[string]interface{}{"status": status}).Error
}

// ListPwdChangesSince returns the accounts whose password changed at or after `since`, with their current version.
// The gateway uses it to rebuild its Redis deny entries after Redis lost its data.
func (r *AccountRepository) ListPwdChangesSince(ctx context.Context, since time.Time) ([]*outbox.PwdVersionKafkaEvent, error) {
	var out []*outbox.PwdVersionKafkaEvent
	err := r.DB.WithContext(ctx).Model(&outbox.PwdVersionEvent{}).Select("user_id", "pwd_version").
		Where("changed_at >= ?", since).Find(&out).Error
	return out, err
}
