package repository

import (
	"auth-service/pkg/model"
	"context"
	"fmt"

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

// GetAccountByUsernameRole get account by username and role
func (r *AccountRepository) GetAccountByUsernameRole(ctx context.Context, username string, role string) (*model.Account, error) {
	var acc model.Account
	if err := r.DB.WithContext(ctx).Model(&model.Account{}).Where("Username = ? and Role = ?", username, role).First(&acc).Error; err != nil {
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
func (r *AccountRepository) UpdatePassword(ctx context.Context, acc *model.Account, newPassword string, pwdVersion int64) error {
	res := r.DB.WithContext(ctx).Model(acc).Select("Password", "PwdVersion").Updates(map[string]interface{}{"Password": newPassword, "PwdVersion": pwdVersion})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("no rows affected")
	}

	return nil
}
