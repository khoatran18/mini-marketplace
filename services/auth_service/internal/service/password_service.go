package service

import (
	"auth-service/pkg/dto"

	"golang.org/x/crypto/bcrypt"
)

// ChangePassword update new password
func (s *AuthService) ChangePassword(req *dto.ChangePasswordInput) (*dto.ChangePasswordOutput, error) {

	// Get account
	acc, err := s.AccountRepo.GetAccountByUsernameRole(req.Username, req.Role)
	if err != nil {
		s.ZapLogger.Warn("AuthService: get account by username role failure")
		return nil, err
	}

	// Check old password
	err = bcrypt.CompareHashAndPassword([]byte(acc.Password), []byte(req.OldPassword))
	if err != nil {
		s.ZapLogger.Warn("AuthService: old password compare failure")
		return nil, err
	}

	// Update password
	hashedNewPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		s.ZapLogger.Warn("AuthService: new password hash failure")
		return nil, err
	}
	newPassword := string(hashedNewPassword)
	newPwdVersion := (acc.PwdVersion + 1) % 100
	err = s.AccountRepo.UpdatePassword(acc, newPassword, newPwdVersion)
	if err != nil {
		s.ZapLogger.Warn("AuthService: update account failure")
		return nil, err
	}

	return &dto.ChangePasswordOutput{
		Message: "Change Password Success",
		Success: true,
	}, nil
}
