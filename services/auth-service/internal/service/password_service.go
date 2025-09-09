package service

import (
	"auth-service/pkg/dto"
	"context"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// ChangePassword update new password
func (s *AuthService) ChangePassword(ctx context.Context, req *dto.ChangePasswordInput) (*dto.ChangePasswordOutput, error) {

	// Get account
	acc, err := s.AccountRepo.GetAccountByUsernameRole(ctx, req.Username, req.Role)
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
	err = s.AccountRepo.UpdatePassword(ctx, acc, newPassword, newPwdVersion)
	if err != nil {
		s.ZapLogger.Warn("AuthService: update account failure", zap.Error(err))
		return nil, err
	}

	// Publish to Kafka to API Gateway
	//topic := "auth.change_password"
	//value := map[string]interface{}{
	//	"id":          acc.ID,
	//	"pwd_version": newPwdVersion,
	//}
	//valueMessage, err := json.Marshal(value)
	//if err != nil {
	//	s.ZapLogger.Warn("AuthService: can not parse to json")
	//}
	//if err := s.MQProducer.Publish(ctx, &kafka.Hash{}, topic, []byte("key"), valueMessage); err != nil {
	//	s.ZapLogger.Warn("AuthService: publish to Kafka failure")
	//}

	return &dto.ChangePasswordOutput{
		Message: "Change Password Success",
		Success: true,
	}, nil
}
