package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	"auth-service/pkg/model"
)

// Account management for the platform administrator. The gateway only lets role "admin" reach these calls; the rules
// that protect the platform itself (no self-delete, never the last admin, staff roles only) live here.

func isStaffRole(role string) bool { return role == RoleAdmin || role == RoleDev }

// ListAccounts returns accounts (all roles) for the admin console.
func (s *AuthService) ListAccounts(ctx context.Context, role, query string, limit int) ([]model.Account, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	return s.AccountRepo.ListAccounts(ctx, role, query, limit)
}

// CreateStaffAccount creates an admin or dev account.
func (s *AuthService) CreateStaffAccount(ctx context.Context, username, password, role string) error {
	if !isStaffRole(role) {
		return status.Error(codes.InvalidArgument, "role must be admin or dev")
	}
	if !credentialPattern.MatchString(username) || !credentialPattern.MatchString(password) {
		return status.Error(codes.InvalidArgument, "username/password must be 3-16 characters of letters, digits or underscore")
	}
	if len(password) < minPasswordLength {
		return status.Errorf(codes.InvalidArgument, "password must be at least %d characters", minPasswordLength)
	}
	existing, err := s.AccountRepo.GetAccountByUsername(ctx, username)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if existing != nil {
		return status.Error(codes.AlreadyExists, "account already exists")
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.AccountRepo.CreateAccount(ctx, &model.Account{Username: username, Password: string(hashed), Role: role}); err != nil {
		return err
	}
	s.ZapLogger.Info("AuthService: staff account created", zap.String("username", username), zap.String("role", role))
	return nil
}

func (s *AuthService) staffTarget(ctx context.Context, id, actorID uint64, what string) (*model.Account, error) {
	acc, err := s.AccountRepo.GetAccountById(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "account not found")
		}
		return nil, err
	}
	if !isStaffRole(acc.Role) {
		return nil, status.Errorf(codes.PermissionDenied, "only admin and dev accounts can be %s here", what)
	}
	if acc.ID == actorID {
		return nil, status.Errorf(codes.FailedPrecondition, "you can not %s your own account", map[string]string{"deleted": "delete", "reset": "reset the password of"}[what])
	}
	return acc, nil
}

// DeleteStaffAccount deletes an admin or dev account. Never yourself, never the last admin.
func (s *AuthService) DeleteStaffAccount(ctx context.Context, id, actorID uint64) error {
	acc, err := s.staffTarget(ctx, id, actorID, "deleted")
	if err != nil {
		return err
	}
	if acc.Role == RoleAdmin {
		n, err := s.AccountRepo.CountByRole(ctx, RoleAdmin)
		if err != nil {
			return err
		}
		if n <= 1 {
			return status.Error(codes.FailedPrecondition, "the last admin account can not be deleted")
		}
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return err
	}
	locked, err := bcrypt.GenerateFromPassword([]byte(hex.EncodeToString(buf)), bcrypt.MinCost)
	if err != nil {
		return err
	}
	if err := s.AccountRepo.DeleteAccount(ctx, acc, string(locked)); err != nil {
		return err
	}
	s.ZapLogger.Info("AuthService: staff account deleted", zap.Uint64("id", id), zap.String("role", acc.Role), zap.Uint64("by", actorID))
	return nil
}

// ResetStaffPassword sets a new password for an admin/dev account (the old one is not needed) and revokes its tokens.
func (s *AuthService) ResetStaffPassword(ctx context.Context, id, actorID uint64, newPassword string) error {
	if len(newPassword) < minPasswordLength {
		return status.Errorf(codes.InvalidArgument, "new password must be at least %d characters", minPasswordLength)
	}
	acc, err := s.staffTarget(ctx, id, actorID, "reset")
	if err != nil {
		return err
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.AccountRepo.UpdatePassword(ctx, acc, string(hashed), (acc.PwdVersion+1)%100); err != nil {
		return fmt.Errorf("reset password: %w", err)
	}
	s.ZapLogger.Info("AuthService: staff password reset", zap.Uint64("id", id), zap.Uint64("by", actorID))
	return nil
}
