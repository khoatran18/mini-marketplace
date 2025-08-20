package service

import (
	"account-service/internal/repository"
	"account-service/pkg/model"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	UserRepo *repository.UserRepository
}

func NewAuthService(userRepo *repository.UserRepository) *AuthService {
	return &AuthService{UserRepo: userRepo}
}

func (s *AuthService) Register(req model.RegisterRequest) error {
	existingUser, err := s.UserRepo.GetUserByName(req.Username)
	if existingUser != nil {
		return errors.New("User already exists")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	newUser := &model.User{
		Username: req.Username,
		Password: string(hashedPassword),
	}

	return s.UserRepo.CreateUser(newUser)

}
