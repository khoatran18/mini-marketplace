package model

import (
	"github.com/golang-jwt/jwt/v5"
)

type AuthClaim struct {
	UserID     uint64
	Username   string
	Role       string
	PwdVersion int64
	Type       string
	jwt.RegisteredClaims
}
