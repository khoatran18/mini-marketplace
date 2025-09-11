package authclient

import (
	"context"
	"user-service/internal/client/clientmanager"
	authpb "user-service/pkg/client/authclient"

	"go.uber.org/zap"
)

type AuthClient struct {
	Client        authpb.AuthServiceClient
	ClientManager *clientmanager.ClientManager
	ZapLogger     *zap.Logger
}

func NewAuthClient(client authpb.AuthServiceClient, cm *clientmanager.ClientManager, logger *zap.Logger) *AuthClient {
	return &AuthClient{
		Client:        client,
		ClientManager: cm,
		ZapLogger:     logger,
	}
}

func (a *AuthClient) GetStoreIDRoleByID(ctx context.Context, input *GetStoreIDRoleByIDInput) (*GetStoreIDRoleByIDOutput, error) {
	if a.Client == nil {
		authClient, err := a.ClientManager.GetOrCreateAuthClient()
		if err != nil {
			a.ZapLogger.Error("AuthClient: AuthClient is nil and create failed", zap.Error(err))
			return nil, err
		}
		a.ZapLogger.Info("AuthClient: AuthClient is nil and create success")
		a.Client = authClient
	}

	res, err := a.Client.GetStoreIDRoleById(ctx, &authpb.GetStoreIDRoleByIDRequest{
		ID: input.ID,
	})
	if err != nil {
		a.ZapLogger.Error("AuthClient: GetStoreIDRoleByID error", zap.Error(err))
		return nil, err
	}

	return &GetStoreIDRoleByIDOutput{
		Message: "Get StoreID, Role successfully",
		Success: true,
		Role:    res.GetRole(),
		StoreID: res.GetStoreId(),
	}, nil
}
