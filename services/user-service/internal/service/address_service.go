package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"user-service/internal/repository"
	"user-service/pkg/model"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var phonePattern = regexp.MustCompile(`^\+?[0-9][0-9 .\-]{7,14}$`)

func tooLong(s string, max int) bool { return len([]rune(s)) > max }

func validateAddress(a *model.Address) error {
	a.Label, a.ReceiverName, a.Phone = strings.TrimSpace(a.Label), strings.TrimSpace(a.ReceiverName), strings.TrimSpace(a.Phone)
	a.Line1, a.Ward, a.District, a.City = strings.TrimSpace(a.Line1), strings.TrimSpace(a.Ward), strings.TrimSpace(a.District), strings.TrimSpace(a.City)
	switch {
	case a.UserID == 0:
		return status.Error(codes.InvalidArgument, "user_id is required")
	case a.ReceiverName == "" || tooLong(a.ReceiverName, 100):
		return status.Error(codes.InvalidArgument, "receiver_name must be 1-100 characters")
	case !phonePattern.MatchString(a.Phone):
		return status.Error(codes.InvalidArgument, "phone is not valid")
	case a.Line1 == "" || tooLong(a.Line1, 200):
		return status.Error(codes.InvalidArgument, "line1 must be 1-200 characters")
	case a.City == "" || tooLong(a.City, 100):
		return status.Error(codes.InvalidArgument, "city must be 1-100 characters")
	case tooLong(a.Label, 50) || tooLong(a.Ward, 100) || tooLong(a.District, 100):
		return status.Error(codes.InvalidArgument, "label, ward or district is too long")
	}
	return nil
}

func addressError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, repository.ErrAddressNotFound):
		return status.Error(codes.NotFound, "address not found")
	case errors.Is(err, repository.ErrTooManyAddress):
		return status.Errorf(codes.FailedPrecondition, "at most %d addresses", model.MaxAddressesPerUser)
	}
	return err
}

// UpsertAddress creates (ID 0) or updates an address of a user.
func (s *UserService) UpsertAddress(ctx context.Context, a *model.Address) (*model.Address, error) {
	if err := validateAddress(a); err != nil {
		return nil, err
	}
	if err := s.UserRepo.UpsertAddress(ctx, a); err != nil {
		return nil, addressError(err)
	}
	return a, nil
}

// ListAddresses lists the user's addresses.
func (s *UserService) ListAddresses(ctx context.Context, userID uint64) ([]*model.Address, error) {
	if userID == 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	return s.UserRepo.ListAddresses(ctx, userID)
}

// GetAddress returns one address of the user (id 0 = default).
func (s *UserService) GetAddress(ctx context.Context, userID, id uint64) (*model.Address, error) {
	if userID == 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	a, err := s.UserRepo.GetAddress(ctx, userID, id)
	return a, addressError(err)
}

// DeleteAddress removes an address of the user.
func (s *UserService) DeleteAddress(ctx context.Context, userID, id uint64) error {
	if userID == 0 || id == 0 {
		return status.Error(codes.InvalidArgument, "user_id and id are required")
	}
	return addressError(s.UserRepo.DeleteAddress(ctx, userID, id))
}
