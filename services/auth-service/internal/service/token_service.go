package service

import (
	"auth-service/pkg/dto"
	"auth-service/pkg/model"
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// generateToken generate token from password
func (s *AuthService) generateToken(ctx context.Context, tokenRequest *dto.TokenRequest) (string, string, error) {
	// Create claims
	accessClaims := &model.AuthClaim{
		UserID:   tokenRequest.UserID,
		Username: tokenRequest.Username,
		Role:     tokenRequest.Role,
		Type:     "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.JWTExpireTime)),
		},
	}
	refreshClaims := &model.AuthClaim{
		UserID:   tokenRequest.UserID,
		Username: tokenRequest.Username,
		Role:     tokenRequest.Role,
		Type:     "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.JWTExpireTime * 2)),
		},
	}

	// Create token
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)

	// Signed token
	signedAccessToken, err := accessToken.SignedString([]byte(s.JWTSecret))
	if err != nil {
		s.ZapLogger.Warn("AuthService: token signed failure")
		return "", "", err
	}
	signedRefreshToken, err := refreshToken.SignedString([]byte(s.JWTSecret))
	if err != nil {
		return "", "", err
	}

	return signedAccessToken, signedRefreshToken, nil
}

// parseToken return claims from token
func (s *AuthService) parseToken(signedToken string) (*model.AuthClaim, error) {

	// Validate token
	token, err := jwt.ParseWithClaims(signedToken, &model.AuthClaim{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			s.ZapLogger.Error("Error unexpected signing method",
				zap.String("alg", token.Header["alg"].(string)),
			)
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.JWTSecret), nil
	})
	if err != nil {
		s.ZapLogger.Warn("AuthService: token parse failure")
		return nil, err
	}
	if !token.Valid {
		s.ZapLogger.Warn("AuthService: token validation failure")
		return nil, err
	}

	// Validate claims
	parsedClaims, ok := token.Claims.(*model.AuthClaim)
	if !ok {
		s.ZapLogger.Warn("AuthService: claims parse failure")
		return nil, fmt.Errorf("failed to cast claims to AuthClaim")
	}

	return parsedClaims, nil
}

// RefreshToken refresh new token
func (s *AuthService) RefreshToken(ctx context.Context, input *dto.RefreshTokenInput) (*dto.RefreshTokenOutput, error) {

	// Parse and validate token
	authClaim, err := s.parseToken(input.RefreshToken)
	if err != nil {
		return nil, err
	}
	if authClaim.Type != "refresh" {
		s.ZapLogger.Warn("AuthService: refresh token only")
		return nil, fmt.Errorf("refresh token only")
	}
	tokenRequest := &dto.TokenRequest{
		UserID:     authClaim.UserID,
		Username:   authClaim.Username,
		Role:       authClaim.Role,
		PwdVersion: authClaim.PwdVersion,
	}

	// Create and sign new token
	signedAccessToken, signedRefreshToken, err := s.generateToken(ctx, tokenRequest)
	if err != nil {
		s.ZapLogger.Warn("AuthService: token generation failure")
		return nil, err
	}
	return &dto.RefreshTokenOutput{
		Message:      "Refresh token successfully",
		AccessToken:  signedAccessToken,
		RefreshToken: signedRefreshToken,
		Success:      true,
	}, nil
}
