package config

import (
	"errors"
	"os"
)

type EnvConfig struct {
	JWTSecret string
}

// InitJWTSecret load env about jwt
func InitJWTSecret() (string, error) {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return "", errors.New("JWT secret not set")
	}
	return jwtSecret, nil
}

// NewEnvConfig load env config
func NewEnvConfig() (*EnvConfig, error) {
	jwtSecret, err := InitJWTSecret()
	if err != nil {
		return nil, err
	}

	return &EnvConfig{
		JWTSecret: jwtSecret,
	}, nil
}
