package repository

import (
	"context"
	"errors"
	"user-service/pkg/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Address errors.
var (
	ErrAddressNotFound = errors.New("address not found")
	ErrTooManyAddress  = errors.New("too many addresses")
)

// ListAddresses returns the user's addresses, default first.
func (r *UserRepository) ListAddresses(ctx context.Context, userID uint64) ([]*model.Address, error) {
	var out []*model.Address
	err := r.DB.WithContext(ctx).Where("user_id = ?", userID).Order("is_default DESC, id ASC").Find(&out).Error
	return out, err
}

// GetAddress returns one address of the user; id 0 means "the default address" (or the oldest when none is default).
func (r *UserRepository) GetAddress(ctx context.Context, userID, id uint64) (*model.Address, error) {
	q := r.DB.WithContext(ctx).Where("user_id = ?", userID)
	if id != 0 {
		q = q.Where("id = ?", id)
	} else {
		q = q.Order("is_default DESC, id ASC")
	}
	var a model.Address
	if err := q.First(&a).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAddressNotFound
		}
		return nil, err
	}
	return &a, nil
}

// UpsertAddress creates (ID 0) or updates an address of a user. The first address becomes the default; setting
// IsDefault clears it on the others, so a user always has at most one default.
func (r *UserRepository) UpsertAddress(ctx context.Context, a *model.Address) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// serialise per user so the count limit and the single default hold under concurrency
		var locked []model.Address
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", a.UserID).Find(&locked).Error; err != nil {
			return err
		}
		if a.ID == 0 {
			if len(locked) >= model.MaxAddressesPerUser {
				return ErrTooManyAddress
			}
			if len(locked) == 0 {
				a.IsDefault = true
			}
			if a.IsDefault {
				if err := tx.Model(&model.Address{}).Where("user_id = ?", a.UserID).Update("is_default", false).Error; err != nil {
					return err
				}
			}
			return tx.Create(a).Error
		}
		var current *model.Address
		for i := range locked {
			if locked[i].ID == a.ID {
				current = &locked[i]
			}
		}
		if current == nil {
			return ErrAddressNotFound
		}
		if a.IsDefault {
			if err := tx.Model(&model.Address{}).Where("user_id = ? AND id <> ?", a.UserID, a.ID).Update("is_default", false).Error; err != nil {
				return err
			}
		} else if current.IsDefault {
			a.IsDefault = true // the default can only be moved by choosing another default
		}
		a.CreatedAt = current.CreatedAt
		return tx.Save(a).Error
	})
}

// DeleteAddress removes an address; when it was the default the oldest remaining one becomes the default.
func (r *UserRepository) DeleteAddress(ctx context.Context, userID, id uint64) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var a model.Address
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", id, userID).First(&a).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAddressNotFound
			}
			return err
		}
		if err := tx.Delete(&a).Error; err != nil {
			return err
		}
		if a.IsDefault {
			var next model.Address
			if err := tx.Where("user_id = ?", userID).Order("id ASC").First(&next).Error; err == nil {
				return tx.Model(&next).Update("is_default", true).Error
			}
		}
		return nil
	})
}
