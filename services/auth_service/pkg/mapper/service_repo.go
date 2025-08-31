package mapper

import (
	"auth-service/pkg/dto"
	"auth-service/pkg/model"
)

func AccountModelToTokenRequest(account *model.Account) *dto.TokenRequest {
	return &dto.TokenRequest{
		UserID:     account.ID,
		Username:   account.Username,
		Role:       account.Role,
		PwdVersion: account.PwdVersion,
	}
}
