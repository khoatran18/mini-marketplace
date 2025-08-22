package model

import "github.com/golang-jwt/jwt/v5"

type AuthClaim struct {
	Username string
	Role     string
	jwt.RegisteredClaims
}
