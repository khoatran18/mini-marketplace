package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

type EnvConfig struct {
	JWTSecret     string
	JWTExpireTime time.Duration
}

// initJWTSecret load env about jwt
func initJWTSecret() (string, error) {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return "", errors.New("JWT secret not set")
	}
	return jwtSecret, nil
}

// initJWTExpireTime load env about jwt expire time
func initJWTExpireTime() (time.Duration, error) {
	jwtExpireTimeStr := os.Getenv("JWT_EXPIRE_TIME")
	jwtExpireTimeMinute, err := strconv.Atoi(jwtExpireTimeStr)
	if err != nil {
		fmt.Println("JWT_EXPIRE_TIME env variable not set, using default 5 minutes")
		jwtExpireTimeMinute = 5
	}

	jwtExpireTime := time.Duration(jwtExpireTimeMinute) * time.Minute

	return jwtExpireTime, nil
}

// NewEnvConfig load env config
func NewEnvConfig() (*EnvConfig, error) {
	jwtSecret, err := initJWTSecret()
	if err != nil {
		return nil, err
	}

	jwtExpireTime, err := initJWTExpireTime()
	if err != nil {
		return nil, err
	}

	return &EnvConfig{
		JWTSecret:     jwtSecret,
		JWTExpireTime: jwtExpireTime,
	}, nil
}
