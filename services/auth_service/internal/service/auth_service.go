package service

import (
	"auth-service/internal/repository"
	"auth-service/pkg/model"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AuthService struct {
	AccountRepo   *repository.AccountRepository
	JWTSecret     string
	JWTExpireTime time.Duration
}

// NewAuthService create new AuthService
func NewAuthService(accountRepo *repository.AccountRepository, jwtSecret string, jwtExpireTime time.Duration) *AuthService {
	return &AuthService{
		AccountRepo:   accountRepo,
		JWTSecret:     jwtSecret,
		JWTExpireTime: jwtExpireTime,
	}
}

// Register handle logic register
func (s *AuthService) Register(req *model.RegisterRequest) error {

	existingAccount, err := s.AccountRepo.GetAccountByUsername(req.Username)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if existingAccount != nil {
		return errors.New("Account already exists")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	newAccount := &model.Account{
		Username: req.Username,
		Password: string(hashedPassword),
		Role:     req.Role,
	}

	return s.AccountRepo.CreateAccount(newAccount)
}

// Login handle logic login
func (s *AuthService) Login(req *model.LoginRequest) (string, error) {
	account, err := s.AccountRepo.GetAccountByUsername(req.Username)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", errors.New("Username or password is incorrect")
	}
	if account == nil {
		return "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(req.Password)); err != nil || account.Role != req.Role {
		return "", errors.New("Username or password is incorrect")
	}

	claims := &model.AuthClaim{
		Username: account.Username,
		Role:     account.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.JWTExpireTime)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(s.JWTSecret))
	if err != nil {
		return "", err
	}

	return signedToken, nil

}
