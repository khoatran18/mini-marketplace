package service

import (
	"auth-service/internal/repository"
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
func (s *AuthService) Register(req *model.RegisterRequest) error {

	// Check account existed
	existingAccount, err := s.AccountRepo.GetAccountByUsernameRole(req.Username, req.Role)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		s.ZapLogger.Error("AuthService: DB error", zap.Error(err))
		return err
	}
	if existingAccount != nil {
		s.ZapLogger.Warn("AuthService: account already exists", zap.String("username", req.Username), zap.String("role", req.Role))
		return errors.New("account already exists")
	}

	// Bcrypt password and create account
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		s.ZapLogger.Warn("AuthService: bcrypt password error", zap.Error(err))
		return err
	}
	newAccount := &model.Account{
		Username:   req.Username,
		Password:   string(hashedPassword),
		Role:       req.Role,
		PwdVersion: 0,
	}

	return s.AccountRepo.CreateAccount(newAccount)
}

// Login handle logic login
func (s *AuthService) Login(req *model.LoginRequest) (string, string, error) {

	// Check account
	account, err := s.AccountRepo.GetAccountByUsernameRole(req.Username, req.Role)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		s.ZapLogger.Warn("AuthService: account not found", zap.String("username", req.Username))
		return "", "", errors.New("username or password is incorrect")
	}
	if account == nil {
		s.ZapLogger.Error("AuthService: DB error", zap.Error(err))
		return "", "", err
	}

	// Check password
	if err := bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(req.Password)); err != nil || account.Role != req.Role {
		s.ZapLogger.Warn("AuthService: wrong password", zap.Error(err))
		return "", "", errors.New("username or password is incorrect")
	}

	// Create token
	tokenRequest := &model.TokenRequest{
		UserID:     account.ID,
		Username:   account.Username,
		Role:       account.Role,
		PwdVersion: account.PwdVersion,
	}
	signedAccessToken, signedRefreshToken, err := s.generateToken(tokenRequest)
	if err != nil {
		s.ZapLogger.Warn("AuthService: token generation failure")
		return "", "", err
	}

	return signedAccessToken, signedRefreshToken, nil
}
