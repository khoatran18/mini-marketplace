package service

import (
	"auth-service/pkg/model"

	"golang.org/x/crypto/bcrypt"
)

// ChangePassword update new password
func (s *AuthService) ChangePassword(req *model.ChangePasswordRequest) error {

	// Get account
	acc, err := s.AccountRepo.GetAccountByUsernameRole(req.Username, req.Role)
	if err != nil {
		s.ZapLogger.Warn("AuthService: get account by username role failure")
		return err
	}

	// Check old password
	err = bcrypt.CompareHashAndPassword([]byte(acc.Password), []byte(req.OldPassword))
	if err != nil {
		s.ZapLogger.Warn("AuthService: old password compare failure")
		return err
	}

	// Update password
	hashedNewPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		s.ZapLogger.Warn("AuthService: new password hash failure")
		return err
	}
	newPassword := string(hashedNewPassword)
	newPwdVersion := (acc.PwdVersion + 1) % 100
	err = s.AccountRepo.UpdatePassword(acc, newPassword, newPwdVersion)
	if err != nil {
		s.ZapLogger.Warn("AuthService: update account failure")
		return err
	}

	return nil
}
