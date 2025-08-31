package service

import (
	"auth-service/internal/repository"
	"auth-service/pkg/dto"
	"auth-service/pkg/model"
	"errors"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// AuthService is responsible for interacting with AuthServer and AccountRepository
type AuthService struct {
	AccountRepo   *repository.AccountRepository
	JWTSecret     string
	JWTExpireTime time.Duration
	ZapLogger     *zap.Logger
}

// NewAuthService create new AuthService
func NewAuthService(accountRepo *repository.AccountRepository, jwtSecret string, jwtExpireTime time.Duration, logger *zap.Logger) *AuthService {
	return &AuthService{
		AccountRepo:   accountRepo,
		JWTSecret:     jwtSecret,
		JWTExpireTime: jwtExpireTime,
		ZapLogger:     logger,
	}
}

// Register handle logic register
func (s *AuthService) Register(input *dto.RegisterInput) (*dto.RegisterOutput, error) {

	// Check account existed
	existingAccount, err := s.AccountRepo.GetAccountByUsernameRole(input.Username, input.Role)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		s.ZapLogger.Error("AuthService: DB error", zap.Error(err))
		return nil, err
	}
	if existingAccount != nil {
		s.ZapLogger.Warn("AuthService: account already exists", zap.String("username", input.Username), zap.String("role", input.Role))
		return nil, errors.New("account already exists")
	}

	// Bcrypt password and create account
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		s.ZapLogger.Warn("AuthService: bcrypt password error", zap.Error(err))
		return nil, err
	}
	newAccount := &model.Account{
		Username:   input.Username,
		Password:   string(hashedPassword),
		Role:       input.Role,
		PwdVersion: 0,
	}

	// Create new account in repository
	err = s.AccountRepo.CreateAccount(newAccount)
	if err != nil {
		s.ZapLogger.Warn("AuthService: failed to create account", zap.Error(err))
		return nil, err
	}

	return &dto.RegisterOutput{
		Message: "Registered successfully",
		Success: true,
	}, nil
}

// Login handle logic login
func (s *AuthService) Login(req *dto.LoginInput) (*dto.LoginOutput, error) {

	// Check account existed
	account, err := s.AccountRepo.GetAccountByUsernameRole(req.Username, req.Role)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		s.ZapLogger.Warn("AuthService: account not found", zap.String("username", req.Username))
		return nil, errors.New("username or password is incorrect")
	}
	if account == nil {
		s.ZapLogger.Error("AuthService: DB error", zap.Error(err))
		return nil, err
	}

	// Check password
	if err := bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(req.Password)); err != nil || account.Role != req.Role {
		s.ZapLogger.Warn("AuthService: wrong password", zap.Error(err))
		return nil, errors.New("username or password is incorrect")
	}

	// Create token
	tokenRequest := AccountModelToTokenRequest(account)
	signedAccessToken, signedRefreshToken, err := s.generateToken(tokenRequest)
	if err != nil {
		s.ZapLogger.Warn("AuthService: token generation failure")
		return nil, err
	}

	return &dto.LoginOutput{
		Message:      "Logged in successfully",
		Success:      true,
		AccessToken:  signedAccessToken,
		RefreshToken: signedRefreshToken,
	}, nil
}
