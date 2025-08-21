package repository

import (
	"auth-service/pkg/model"

	"gorm.io/gorm"
)

type AccountRepository struct {
	DB *gorm.DB
}

// NewAccountRepository create new AccountRepository
func NewAccountRepository(db *gorm.DB) *AccountRepository {
	return &AccountRepository{DB: db}
}

// CreateAccount create new account
func (r *AccountRepository) CreateAccount(user *model.Account) error {
	return r.DB.Create(user).Error
}

// GetAccountByUsername get account by username
func (r *AccountRepository) GetAccountByUsername(username string) (*model.Account, error) {
	var user model.Account
	if err := r.DB.Where("Username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}

	return &user, nil
}

// GetAccountByUsernamePasswordRole get account, mainly for logic login
func (r *AccountRepository) GetAccountByUsernamePasswordRole(username, password, role string) (*model.Account, error) {
	var user model.Account
	if err := r.DB.Where("Username = ? and Password = ? and Role = ?", username, password, role).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
