package authclient

import (
	"api-gateway/pkg/dto"
	authpb "api-gateway/pkg/pb/authservice"
	"context"
	"time"

	"buf.build/go/protovalidate"
)

// Account management calls (platform admin). Requests are validated with the proto rules before they leave the gateway.

func (s *AuthClient) ensure() error {
	if s.Client == nil {
		return s.validateClient()
	}
	return nil
}

func (s *AuthClient) ListAccounts(ctx context.Context, role, query string, limit uint32) (*dto.ListAccountsOutput, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	req := &authpb.ListAccountsRequest{Role: role, Query: query, Limit: limit}
	if err := protovalidate.Validate(req); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := s.Client.ListAccounts(ctx, req)
	if err != nil {
		return nil, err
	}
	out := &dto.ListAccountsOutput{Accounts: make([]dto.AccountInfo, 0, len(res.GetAccounts()))}
	for _, a := range res.GetAccounts() {
		out.Accounts = append(out.Accounts, dto.AccountInfo{ID: a.GetId(), Username: a.GetUsername(), Role: a.GetRole(), StoreID: a.GetStoreId()})
	}
	return out, nil
}

func (s *AuthClient) CreateStaffAccount(ctx context.Context, in *dto.CreateStaffInput) (*dto.RegisterOutput, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	req := &authpb.CreateStaffAccountRequest{Username: in.Username, Password: in.Password, Role: in.Role}
	if err := protovalidate.Validate(req); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := s.Client.CreateStaffAccount(ctx, req)
	if err != nil {
		return nil, err
	}
	return &dto.RegisterOutput{Message: res.GetMessage(), Success: res.GetSuccess()}, nil
}

func (s *AuthClient) DeleteStaffAccount(ctx context.Context, id, actorID uint64) (*dto.RegisterOutput, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	req := &authpb.StaffAccountRequest{Id: id, ActorId: actorID}
	if err := protovalidate.Validate(req); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := s.Client.DeleteStaffAccount(ctx, req)
	if err != nil {
		return nil, err
	}
	return &dto.RegisterOutput{Message: res.GetMessage(), Success: res.GetSuccess()}, nil
}

func (s *AuthClient) ResetStaffPassword(ctx context.Context, id, actorID uint64, newPassword string) (*dto.RegisterOutput, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	req := &authpb.StaffAccountRequest{Id: id, ActorId: actorID, NewPassword: newPassword}
	if err := protovalidate.Validate(req); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := s.Client.ResetStaffPassword(ctx, req)
	if err != nil {
		return nil, err
	}
	return &dto.RegisterOutput{Message: res.GetMessage(), Success: res.GetSuccess()}, nil
}
