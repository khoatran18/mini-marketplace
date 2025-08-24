package model

import (
	"github.com/golang-jwt/jwt/v5"
)

type AuthClaim struct {
	UserID     uint
	Username   string
	Role       string
	PwdVersion int
	Type       string
	jwt.RegisteredClaims
}

type TokenRequest struct {
	UserID     uint
	Username   string
	Role       string
	PwdVersion int
}
