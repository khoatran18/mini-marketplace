package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type EnvConfig struct {
	JWTSecret      string
	JWTExpireTime  time.Duration
	AllowedOrigins []string
	// RateLimit is the max requests per minute per client IP (0 disables the limiter).
	RateLimit int
	// AuthRateLimit is the stricter per-minute limit applied to /auth routes.
	AuthRateLimit  int
	TrustedProxies []string
}

// InitJWTSecret load env about jwt
func InitJWTSecret() (string, error) {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return "", errors.New("JWT secret not set")
	}
	if len(jwtSecret) < 16 {
		return "", errors.New("JWT secret is too short (min 16 characters)")
	}
	return jwtSecret, nil
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return def
}

func envList(key string, def []string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	var out []string
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// NewEnvConfig load env config
func NewEnvConfig() (*EnvConfig, error) {
	jwtSecret, err := InitJWTSecret()
	if err != nil {
		return nil, err
	}

	return &EnvConfig{
		JWTSecret:      jwtSecret,
		JWTExpireTime:  time.Duration(envInt("JWT_EXPIRE_TIME", 5)) * time.Minute,
		AllowedOrigins: envList("ALLOWED_ORIGINS", []string{"http://localhost:3000"}),
		RateLimit:      envInt("RATE_LIMIT_PER_MINUTE", 300),
		AuthRateLimit:  envInt("AUTH_RATE_LIMIT_PER_MINUTE", 20),
		TrustedProxies: envList("TRUSTED_PROXIES", nil),
	}, nil
}
