package main

import (
	"auth-service/internal/server"
	authpb "auth-service/pkg/pb"
	"context"
	"fmt"
)

func SeedAccounts(s *server.AuthServer) {
	ctx := context.Background()

	accounts := []struct {
		Username string
		Password string
		Role     string
	}{
		{"buyer1", "password", "buyer"},
		{"seller1", "password", "seller_admin"},
	}

	for _, acc := range accounts {
		_, err := s.Register(ctx, &authpb.RegisterRequest{
			Username: acc.Username,
			Password: acc.Password,
			Role:     acc.Role,
		})
		if err != nil {
			fmt.Printf("❌ Seed account %s thất bại: %v\n", acc.Username, err)
			continue
		}
		fmt.Printf("✅ Seed account %s thành công\n", acc.Username)
	}
}
