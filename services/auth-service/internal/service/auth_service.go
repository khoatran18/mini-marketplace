package service

import (
	"auth-service/internal/config/messagequeue"
	"auth-service/internal/config/messagequeue/kafkaimpl"
	"auth-service/internal/repository"
	"auth-service/internal/service/adapter"
	"auth-service/pkg/dto"
	"auth-service/pkg/model"
	"context"
	"errors"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"log"
	"regexp"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const minPasswordLength = 8

// validRoles are the roles that can be registered through the API. "admin" is deliberately not
// in this list: administrators are only created by EnsureAdmin (ADMIN_BOOTSTRAP_* at start-up).
var validRoles = map[string]bool{"buyer": true, "seller_admin": true, "seller_employee": true}

// RoleAdmin is the platform administrator role (cannot be self-registered).
const RoleAdmin = "admin"

// credentialPattern mirrors the login contract in auth.proto (username/password: 3-16 letters, digits, underscore).
var credentialPattern = regexp.MustCompile(`^[a-zA-Z0-9_]{3,16}$`)

// AuthService is responsible for interacting with AuthServer and AccountRepository
type AuthService struct {
	AccountRepo   *repository.AccountRepository
	JWTSecret     string
	JWTExpireTime time.Duration
	MQProducer    messagequeue.Producer
	MQConsumer    messagequeue.Consumer
	KafkaClient   *kafkaimpl.KafkaClient
	ZapLogger     *zap.Logger
}

// NewAuthService create new AuthService
func NewAuthService(accountRepo *repository.AccountRepository, jwtSecret string, jwtExpireTime time.Duration, logger *zap.Logger,
	producer messagequeue.Producer, consumer messagequeue.Consumer, kafkaClient *kafkaimpl.KafkaClient) *AuthService {

	return &AuthService{
		AccountRepo:   accountRepo,
		JWTSecret:     jwtSecret,
		JWTExpireTime: jwtExpireTime,
		MQProducer:    producer,
		MQConsumer:    consumer,
		KafkaClient:   kafkaClient,
		ZapLogger:     logger,
	}
}

// Register handle logic register
func (s *AuthService) Register(ctx context.Context, input *dto.RegisterInput) (*dto.RegisterOutput, error) {

	// Validate role and password
	if !validRoles[input.Role] {
		return nil, status.Error(codes.InvalidArgument, "invalid role")
	}
	if input.Role == input.RoleNotRegister {
		return nil, status.Errorf(codes.PermissionDenied, "can not register role %s", input.Role)
	}
	if len(input.Password) < minPasswordLength {
		return nil, status.Errorf(codes.InvalidArgument, "password must be at least %d characters", minPasswordLength)
	}

	// Check account existed
	existingAccount, err := s.AccountRepo.GetAccountByUsernameRole(ctx, input.Username, input.Role)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		s.ZapLogger.Error("AuthService: DB error", zap.Error(err))
		return nil, err
	}
	if existingAccount != nil {
		s.ZapLogger.Warn("AuthService: account already exists", zap.String("username", input.Username), zap.String("role", input.Role))
		return nil, status.Error(codes.AlreadyExists, "account already exists")
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
		StoreID:    input.StoreID,
		PwdVersion: 0,
	}

	// Create new account in repository
	err = s.AccountRepo.CreateAccount(ctx, newAccount)
	if err != nil {
		s.ZapLogger.Warn("AuthService: failed to create account", zap.Error(err))
		return nil, err
	}

	return &dto.RegisterOutput{
		Message: "Registered successfully",
		Success: true,
	}, nil
}

// EnsureAdmin creates the platform administrator if no account with that username and role "admin"
// exists yet. It is idempotent and never changes the password of an existing admin. The credentials
// must satisfy the same rules as login (auth.proto), otherwise the admin could never sign in.
func (s *AuthService) EnsureAdmin(ctx context.Context, username, password string) (created bool, err error) {
	if !credentialPattern.MatchString(username) || !credentialPattern.MatchString(password) {
		return false, errors.New("admin username/password must be 3-16 characters of letters, digits or underscore")
	}
	if len(password) < minPasswordLength {
		return false, fmt.Errorf("admin password must be at least %d characters", minPasswordLength)
	}
	existing, err := s.AccountRepo.GetAccountByUsernameRole(ctx, username, RoleAdmin)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	if existing != nil {
		return false, nil
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return false, err
	}
	if err := s.AccountRepo.CreateAccount(ctx, &model.Account{Username: username, Password: string(hashed), Role: RoleAdmin}); err != nil {
		return false, err
	}
	s.ZapLogger.Info("AuthService: admin account created", zap.String("username", username))
	return true, nil
}

// Login handle logic login
func (s *AuthService) Login(ctx context.Context, req *dto.LoginInput) (*dto.LoginOutput, error) {

	// Check account existed
	account, err := s.AccountRepo.GetAccountByUsernameRole(ctx, req.Username, req.Role)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		s.ZapLogger.Warn("AuthService: account not found", zap.String("username", req.Username))
		return nil, status.Error(codes.Unauthenticated, "username or password is incorrect")
	}
	if account == nil {
		s.ZapLogger.Error("AuthService: DB error", zap.Error(err))
		return nil, err
	}

	// Check password
	if err := bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(req.Password)); err != nil || account.Role != req.Role {
		s.ZapLogger.Warn("AuthService: wrong password", zap.Error(err))
		return nil, status.Error(codes.Unauthenticated, "username or password is incorrect")
	}

	// Create token
	tokenRequest := adapter.AccountModelToTokenRequest(account)
	signedAccessToken, signedRefreshToken, err := s.generateToken(ctx, tokenRequest)
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

// RegisterSellerRoles handle logic create accounts for seller_admin or seller_employee role
func (s *AuthService) RegisterSellerRoles(ctx context.Context, input *dto.RegisterSellerRolesInput) (*dto.RegisterSellerRolesOutput, error) {

	// Employees are the only role a seller admin may create
	if input.Role != "seller_employee" {
		return nil, status.Error(codes.InvalidArgument, "only seller_employee accounts can be created")
	}

	// Validate Role and Account
	acc, err := s.AccountRepo.GetAccountById(ctx, input.SellerAdminID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		s.ZapLogger.Warn("AuthService: account not found")
		return nil, status.Error(codes.NotFound, "user_id is invalid")
	}
	if acc == nil {
		s.ZapLogger.Error("AuthService: DB error", zap.Error(err))
		return nil, err
	}

	// Only SellerAdmin have Store can create this role
	if acc.Role != "seller_admin" || acc.StoreID == 0 {
		return nil, status.Error(codes.PermissionDenied, "this account can not take this action")
	}

	log.Printf("Store ID: %d\n", acc.StoreID)
	// Register new account
	subOutput, err := s.Register(ctx, &dto.RegisterInput{
		Username:        input.Username,
		Password:        input.Password,
		Role:            input.Role,
		StoreID:         acc.StoreID,
		RoleNotRegister: "buyer",
	})
	if err != nil {
		return nil, err
	}
	if !subOutput.Success {
		return nil, errors.New("register failed with internal error")
	}

	return &dto.RegisterSellerRolesOutput{
		Success: true,
		Message: "Registered successfully",
	}, nil
}

func (s *AuthService) GetStoreIDRoleById(ctx context.Context, input *dto.GetStoreIDRoleByIdInput) (*dto.GetStoreIDRoleByIdOutput, error) {
	acc, err := s.AccountRepo.GetAccountById(ctx, input.ID)
	if err != nil {
		s.ZapLogger.Warn("AuthService: Get account error", zap.Error(err))
		return nil, err
	}
	return &dto.GetStoreIDRoleByIdOutput{
		Message: acc.Username,
		Success: true,
		Role:    acc.Role,
		StoreID: acc.StoreID,
	}, nil

}
