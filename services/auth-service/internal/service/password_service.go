package service

import (
	"auth-service/pkg/dto"
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// ChangePassword update new password
func (s *AuthService) ChangePassword(ctx context.Context, req *dto.ChangePasswordInput) (*dto.ChangePasswordOutput, error) {

	// Get account
	if len(req.NewPassword) < minPasswordLength {
		return nil, status.Errorf(codes.InvalidArgument, "new password must be at least %d characters", minPasswordLength)
	}
	acc, err := s.AccountRepo.GetAccountByUsernameRole(ctx, req.Username, req.Role)
	if err != nil {
		s.ZapLogger.Warn("AuthService: get account by username role failure")
		return nil, status.Error(codes.NotFound, "account not found")
	}

	// Check old password
	err = bcrypt.CompareHashAndPassword([]byte(acc.Password), []byte(req.OldPassword))
	if err != nil {
		s.ZapLogger.Warn("AuthService: old password compare failure")
		return nil, status.Error(codes.InvalidArgument, "old password is incorrect")
	}
	// Update password
	hashedNewPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		s.ZapLogger.Warn("AuthService: new password hash failure")
		return nil, err
	}
	newPassword := string(hashedNewPassword)
	newPwdVersion := (acc.PwdVersion + 1) % 100
	err = s.AccountRepo.UpdatePassword(ctx, acc, newPassword, newPwdVersion)
	if err != nil {
		s.ZapLogger.Warn("AuthService: update account failure", zap.Error(err))
		return nil, err
	}

	return &dto.ChangePasswordOutput{
		Message: "Change Password Success",
		Success: true,
	}, nil
}
