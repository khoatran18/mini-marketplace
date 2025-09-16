package adapter

import (
	"user-service/pkg/dto"
	userpb "user-service/pkg/pb"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func BuyerProtoToDTO(buyer *userpb.Buyer) (*dto.Buyer, error) {
	return &dto.Buyer{
		UserID:      buyer.GetUserId(),
		Name:        buyer.GetName(),
		Gender:      buyer.GetGender(),
		DateOfBirth: buyer.GetDateOfBirth().AsTime(),
		Phone:       buyer.GetPhone(),
		Address:     buyer.GetAddress(),
	}, nil
}
func BuyerDTOToProto(input *dto.Buyer) (*userpb.Buyer, error) {
	return &userpb.Buyer{
		UserId:      input.UserID,
		Name:        input.Name,
		Gender:      input.Gender,
		DateOfBirth: timestamppb.New(input.DateOfBirth),
		Phone:       input.Phone,
		Address:     input.Address,
	}, nil
}

func SellerProtoToDTO(seller *userpb.Seller) (*dto.Seller, error) {
	return &dto.Seller{
		ID:          seller.GetId(),
		Name:        seller.GetName(),
		BankAccount: seller.GetBankAccount(),
		TaxCode:     seller.GetTaxCode(),
		Description: seller.GetDescription(),
		DateOfBirth: seller.GetDateOfBirth().AsTime(),
		Phone:       seller.GetPhone(),
		Address:     seller.GetAddress(),
	}, nil
}

func SellerDTOToProto(input *dto.Seller) (*userpb.Seller, error) {
	return &userpb.Seller{
		Id:          input.ID,
		Name:        input.Name,
		BankAccount: input.BankAccount,
		TaxCode:     input.TaxCode,
		Description: input.Description,
		DateOfBirth: timestamppb.New(input.DateOfBirth),
		Phone:       input.Phone,
		Address:     input.Address,
	}, nil
}

func CreBuyRequestToInput(req *userpb.CreateBuyerRequest) (*dto.CreateBuyerInput, error) {
	buyer, err := BuyerProtoToDTO(req.GetBuyer())
	if err != nil {
		return nil, err
	}
	return &dto.CreateBuyerInput{
		Buyer: buyer,
	}, nil
}
func CreBuyOutputToResponse(output *dto.CreateBuyerOutput) (*userpb.CreateBuyerResponse, error) {
	return &userpb.CreateBuyerResponse{
		Message: output.Message,
		Success: output.Success,
	}, nil
}

func UpdBuyByUseIDRequestToInput(req *userpb.UpdateBuyerByUserIDRequest) (*dto.UpdateBuyerByUserIDInput, error) {
	buyer, err := BuyerProtoToDTO(req.GetBuyer())
	if err != nil {
		return nil, err
	}
	return &dto.UpdateBuyerByUserIDInput{
		Buyer: buyer,
	}, nil
}
func UpdBuyByUseIDOutputToResponse(output *dto.UpdateBuyerByUserIDOutput) (*userpb.UpdateBuyerByUserIDResponse, error) {
	return &userpb.UpdateBuyerByUserIDResponse{
		Message: output.Message,
		Success: output.Success,
	}, nil
}

func GetBuyByUseIDRequestToInput(req *userpb.GetBuyerByUserIDRequest) (*dto.GetBuyerByUserIDInput, error) {
	return &dto.GetBuyerByUserIDInput{
		UserID: req.GetUserId(),
	}, nil
}
func GetBuyByUseIDOutputToResponse(output *dto.GetBuyerByUserIDOutput) (*userpb.GetBuyerByUserIDResponse, error) {
	buyer, err := BuyerDTOToProto(output.Buyer)
	if err != nil {
		return nil, err
	}
	return &userpb.GetBuyerByUserIDResponse{
		Message: output.Message,
		Success: output.Success,
		Buyer:   buyer,
	}, nil
}

func DelBuyByUseIDRequestToInput(req *userpb.DelBuyerByUserIDRequest) (*dto.DelBuyerByUserIDInput, error) {
	return &dto.DelBuyerByUserIDInput{
		UserID: req.GetUserId(),
	}, nil
}
func DelBuyByUseIDOutputToResponse(output *dto.DelBuyerByUserIDOutput) (*userpb.DelBuyerByUserIDResponse, error) {
	return &userpb.DelBuyerByUserIDResponse{
		Message: output.Message,
		Success: output.Success,
	}, nil
}

// Adapter for Seller

func CreSelRequestToInput(req *userpb.CreateSellerRequest) (*dto.CreateSellerInput, error) {
	buyer, err := SellerProtoToDTO(req.GetSeller())
	if err != nil {
		return nil, err
	}
	return &dto.CreateSellerInput{
		Seller: buyer,
		UserID: req.UserId,
	}, nil
}
func CreSelOutputToResponse(output *dto.CreateSellerOutput) (*userpb.CreateSellerResponse, error) {
	return &userpb.CreateSellerResponse{
		Message: output.Message,
		Success: output.Success,
	}, nil
}

func UpdSelByIDRequestToInput(req *userpb.UpdateSellerByIDRequest) (*dto.UpdateSellerByIDInput, error) {
	buyer, err := SellerProtoToDTO(req.GetSeller())
	if err != nil {
		return nil, err
	}
	return &dto.UpdateSellerByIDInput{
		Seller: buyer,
		UserID: req.GetUserID(),
	}, nil
}
func UpdSelByIDOutputToResponse(output *dto.UpdateSellerByIDOutput) (*userpb.UpdateSellerByIDResponse, error) {
	return &userpb.UpdateSellerByIDResponse{
		Message: output.Message,
		Success: output.Success,
	}, nil
}

func GetSelByIDRequestToInput(req *userpb.GetSellerByIDRequest) (*dto.GetSellerByIDInput, error) {
	return &dto.GetSellerByIDInput{
		UserID: req.GetUserId(),
	}, nil
}
func GetSelByIDOutputToResponse(output *dto.GetSellerByIDOutput) (*userpb.GetSellerByIDResponse, error) {
	buyer, err := SellerDTOToProto(output.Seller)
	if err != nil {
		return nil, err
	}
	return &userpb.GetSellerByIDResponse{
		Message: output.Message,
		Success: output.Success,
		Seller:  buyer,
	}, nil
}

func DelSelByIDRequestToInput(req *userpb.DelSellerByIDRequest) (*dto.DelSellerByIDInput, error) {
	return &dto.DelSellerByIDInput{
		UserID: req.GetUserId(),
	}, nil
}
func DelSelByIDOutputToResponse(output *dto.DelSellerByIDOutput) (*userpb.DelSellerByIDResponse, error) {
	return &userpb.DelSellerByIDResponse{
		Message: output.Message,
		Success: output.Success,
	}, nil
}
