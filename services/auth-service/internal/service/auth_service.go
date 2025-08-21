package service

import (
	"account-service/internal/repository"
	"account-service/pkg/model"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AuthService struct {
	UserRepo      *repository.UserRepository
	JWTSecret     string
	JWTExpireTime time.Duration
}

func NewAuthService(userRepo *repository.UserRepository, jwtSecret string, jwtExpireTime time.Duration) *AuthService {
	return &AuthService{
		UserRepo:      userRepo,
		JWTSecret:     jwtSecret,
		JWTExpireTime: jwtExpireTime,
	}
}

func (s *AuthService) Register(req model.RegisterRequest) error {

	existingUser, err := s.UserRepo.GetUserByUsername(req.Username)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
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

func (s *AuthService) Login(req model.LoginRequest) (string, error) {
	user, err := s.UserRepo.GetUserByUsername(req.Username)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", errors.New("Username or password is incorrect")
	}
	if user == nil {
		return "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return "", errors.New("Username or password is incorrect")
	}

	claims := &model.AuthClaim{
		Username: user.Username,
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
