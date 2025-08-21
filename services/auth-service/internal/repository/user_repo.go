package repository

import (
	"account-service/pkg/model"

	"gorm.io/gorm"
)

type UserRepository struct {
	DB *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{DB: db}
}

func (r *UserRepository) CreateUser(user *model.User) error {
	return r.DB.Create(user).Error
}

func (r *UserRepository) GetUserByUsername(username string) (*model.User, error) {
	var user model.User
	if err := r.DB.Where("Username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *UserRepository) GetUserByUsernameAndPassword(username, password string) (*model.User, error) {
	var user model.User
	if err := r.DB.Where("Username = ? and Password = ?", username, password).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
