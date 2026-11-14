package repository

import (
	"auth-service/pkg/model"
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// AccountRepository is responsible for interacting with account Database
type AccountRepository struct {
	DB *gorm.DB
}

// NewAccountRepository create new AccountRepository
func NewAccountRepository(db *gorm.DB) *AccountRepository {
	return &AccountRepository{DB: db}
}

// CreateAccount create new account
func (r *AccountRepository) CreateAccount(ctx context.Context, user *model.Account) error {
	return r.DB.WithContext(ctx).Create(user).Error
}

// GetAccountById get Account by UserID
func (r *AccountRepository) GetAccountById(ctx context.Context, userID uint64) (*model.Account, error) {
	acc := &model.Account{}
	if err := r.DB.WithContext(ctx).First(acc, userID).Error; err != nil {
		return nil, err
	}
	return acc, nil
}

// GetAccountByUsernameRole get account by username and role
func (r *AccountRepository) GetAccountByUsernameRole(ctx context.Context, username string, role string) (*model.Account, error) {
	var acc model.Account
	if err := r.DB.WithContext(ctx).Model(&model.Account{}).Where("Username = ? and Role = ?", username, role).First(&acc).Error; err != nil {
		return nil, err
	}

	return &acc, nil
}

// GetAccountByUsername returns the account for a username (usernames are unique across roles since migration 00003).
// Legacy databases that still hold the same username under several roles get the oldest account.
func (r *AccountRepository) GetAccountByUsername(ctx context.Context, username string) (*model.Account, error) {
	var acc model.Account
	if err := r.DB.WithContext(ctx).Model(&model.Account{}).Where("Username = ?", username).Order("id ASC").First(&acc).Error; err != nil {
		return nil, err
	}
	return &acc, nil
}

// GetAccountByUsernamePasswordRole get account, mainly for logic login
func (r *AccountRepository) GetAccountByUsernamePasswordRole(ctx context.Context, username, password, role string) (*model.Account, error) {
	var user model.Account
	if err := r.DB.WithContext(ctx).Model(&model.Account{}).Where("Username = ? and Password = ? and Role = ?", username, password, role).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// UpdatePassword update password
//func (r *AccountRepository) UpdatePassword(ctx context.Context, acc *model.Account, newPassword string, pwdVersion int64) error {
//	res := r.DB.WithContext(ctx).Model(acc).Select("Password", "PwdVersion").
//		Updates(map[string]interface{}{"Password": newPassword, "PwdVersion": pwdVersion})
//	if res.Error != nil {
//		return res.Error
//	}
//	if res.RowsAffected == 0 {
//		return fmt.Errorf("no rows affected")
//	}
//
//	return nil
//}

// UpdatePassword update password and insert into OutboxDB for Kafka Producer
func (r *AccountRepository) UpdatePassword(ctx context.Context, acc *model.Account, newPassword string, pwdVersion int64) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {

		// Update in Account model
		res := tx.Model(acc).Select("Password", "PwdVersion").
			Updates(map[string]interface{}{"Password": newPassword, "PwdVersion": pwdVersion})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("no rows affected")
		}

		// Update in Outbox DB
		if err := r.CreateOrUpdatePwdVersionEvent(tx, acc.ID, pwdVersion); err != nil {
			return err
		}

		return nil
	})
}

func (r *AccountRepository) UpdateStoreID(ctx context.Context, id uint64, storeID uint64) error {
	result := r.DB.WithContext(ctx).Model(&model.Account{}).Where("id = ?", id).Updates(map[string]interface{}{"store_id": storeID})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("no rows affected")
	}
	return nil
}

// ListAccounts returns accounts ordered by id, optionally filtered by role and a username substring.
func (r *AccountRepository) ListAccounts(ctx context.Context, role, query string, limit int) ([]model.Account, error) {
	q := r.DB.WithContext(ctx).Model(&model.Account{})
	if role != "" {
		q = q.Where("role = ?", role)
	}
	if query != "" {
		q = q.Where("username ILIKE ?", "%"+escapeLike(query)+"%")
	}
	var out []model.Account
	if err := q.Order("id ASC").Limit(limit).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// CountByRole counts the live accounts of a role.
func (r *AccountRepository) CountByRole(ctx context.Context, role string) (int64, error) {
	var n int64
	err := r.DB.WithContext(ctx).Model(&model.Account{}).Where("role = ?", role).Count(&n).Error
	return n, err
}

// DeleteAccount removes an account for good (staff accounts own no data elsewhere, and a soft-deleted row would keep the
// username taken). In the same transaction the password version is bumped, so tokens issued earlier stop working
// everywhere as soon as the outbox event is published.
func (r *AccountRepository) DeleteAccount(ctx context.Context, acc *model.Account, lockedPassword string) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		next := (acc.PwdVersion + 1) % 100
		if err := tx.Model(acc).Select("Password", "PwdVersion").Updates(map[string]interface{}{"Password": lockedPassword, "PwdVersion": next}).Error; err != nil {
			return err
		}
		if err := r.CreateOrUpdatePwdVersionEvent(tx, acc.ID, next); err != nil {
			return err
		}
		return tx.Unscoped().Delete(&model.Account{}, acc.ID).Error
	})
}
