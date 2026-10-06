package server

import (
	"context"
	"user-service/pkg/model"
	userpb "user-service/pkg/pb"
)

func addressToProto(a *model.Address) *userpb.Address {
	if a == nil {
		return nil
	}
	return &userpb.Address{
		Id: a.ID, UserId: a.UserID, Label: a.Label, ReceiverName: a.ReceiverName, Phone: a.Phone, Line1: a.Line1,
		Ward: a.Ward, District: a.District, City: a.City, IsDefault: a.IsDefault,
	}
}

// UpsertAddress creates or updates an address.
func (s *UserServer) UpsertAddress(ctx context.Context, req *userpb.UpsertAddressRequest) (*userpb.AddressResponse, error) {
	in := req.GetAddress()
	a, err := s.UserService.UpsertAddress(ctx, &model.Address{
		ID: in.GetId(), UserID: in.GetUserId(), Label: in.GetLabel(), ReceiverName: in.GetReceiverName(), Phone: in.GetPhone(),
		Line1: in.GetLine1(), Ward: in.GetWard(), District: in.GetDistrict(), City: in.GetCity(), IsDefault: in.GetIsDefault(),
	})
	if err != nil {
		return nil, err
	}
	return &userpb.AddressResponse{Message: "ok", Success: true, Address: addressToProto(a)}, nil
}

// ListAddresses lists the user's addresses.
func (s *UserServer) ListAddresses(ctx context.Context, req *userpb.ListAddressesRequest) (*userpb.ListAddressesResponse, error) {
	list, err := s.UserService.ListAddresses(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}
	out := make([]*userpb.Address, 0, len(list))
	for _, a := range list {
		out = append(out, addressToProto(a))
	}
	return &userpb.ListAddressesResponse{Message: "ok", Success: true, Addresses: out}, nil
}

// GetAddress returns one address (id 0 = default).
func (s *UserServer) GetAddress(ctx context.Context, req *userpb.GetAddressRequest) (*userpb.AddressResponse, error) {
	a, err := s.UserService.GetAddress(ctx, req.GetUserId(), req.GetId())
	if err != nil {
		return nil, err
	}
	return &userpb.AddressResponse{Message: "ok", Success: true, Address: addressToProto(a)}, nil
}

// DeleteAddress removes an address.
func (s *UserServer) DeleteAddress(ctx context.Context, req *userpb.DeleteAddressRequest) (*userpb.DeleteAddressResponse, error) {
	if err := s.UserService.DeleteAddress(ctx, req.GetUserId(), req.GetId()); err != nil {
		return nil, err
	}
	return &userpb.DeleteAddressResponse{Message: "ok", Success: true}, nil
}
