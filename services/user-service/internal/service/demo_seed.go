package service

import (
	"context"
	"errors"
	"time"
	"user-service/internal/client/authclient"
	"user-service/pkg/model"

	"go.uber.org/zap"
)

// Demo store: product-service's demo catalog is created with seller_id = DemoStoreID, and auth-service's demo
// account "seller1" (the first seller_admin without a store among the first account ids) has to own that store.
const (
	DemoStoreID       uint64 = 2
	demoAccountScanTo uint64 = 10 // account ids 1..10 are scanned for the demo seller_admin (admin bootstrap may shift ids)
)

// SeedDemoSeller (SEED_DEMO_DATA=true only) creates the store of the demo seller so that the demo products belong
// to a real store and "seller1" can manage them. auth-service may still be starting, so it retries for a while.
// It never touches an installation that already has a seller.
func (s *UserService) SeedDemoSeller(ctx context.Context) {
	go func() {
		for attempt := 0; attempt < 30; attempt++ {
			ok, err := s.seedDemoSellerOnce(ctx)
			if err == nil {
				if ok {
					s.ZapLogger.Info("UserService: demo store created", zap.Uint64("store_id", DemoStoreID))
				}
				return
			}
			s.ZapLogger.Warn("UserService: demo store seed will retry", zap.Error(err))
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()
}

func (s *UserService) seedDemoSellerOnce(ctx context.Context) (bool, error) {
	var lastErr error
	for id := uint64(1); id <= demoAccountScanTo; id++ {
		acc, err := s.SCM.AuthServiceClient.GetStoreIDRoleByID(ctx, &authclient.GetStoreIDRoleByIDInput{ID: id})
		if err != nil {
			lastErr = err // a missing account comes back as an error too; the caller retries the whole scan
			continue
		}
		if acc.Role != "seller_admin" {
			continue
		}
		if acc.StoreID != 0 {
			return false, nil // the demo seller already has a store
		}
		return s.UserRepo.SeedDemoSeller(ctx, &model.Seller{
			ID: DemoStoreID, Name: "Demo Store", BankAccount: "0000000000", TaxCode: "0000000000",
			Description: "Demo seller (SEED_DEMO_DATA)", Phone: "0900000000", Address: "1 Demo Street, Ho Chi Minh City",
		}, id)
	}
	if lastErr == nil {
		lastErr = errors.New("demo seller_admin account not found yet")
	}
	return false, lastErr
}
