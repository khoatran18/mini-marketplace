package userclient

import (
	"api-gateway/pkg/dto"
	userpb "api-gateway/pkg/pb/userservice"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func BuyerProtoToDTO(buyer *userpb.Buyer) (*dto.Buyer, error) {
	if buyer == nil {
		return nil, nil
	}
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
	if input == nil {
		return nil, nil
	}
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
	if seller == nil {
		return nil, nil
	}
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
	if input == nil {
		return nil, nil
	}
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

func CreBuyInputToRequest(input *dto.CreateBuyerInput) (*userpb.CreateBuyerRequest, error) {
	buyer, err := BuyerDTOToProto(input.Buyer)
	if err != nil {
		return nil, err
	}
	return &userpb.CreateBuyerRequest{
		Buyer: buyer,
	}, nil
}
func CreBuyResponseToOutput(res *userpb.CreateBuyerResponse) (*dto.CreateBuyerOutput, error) {
	return &dto.CreateBuyerOutput{
		Message: res.GetMessage(),
		Success: res.GetSuccess(),
	}, nil
}

func UpdBuyByUseIDInputToRequest(input *dto.UpdBuyByUseIDInput) (*userpb.UpdateBuyerByUserIDRequest, error) {
	buyer, err := BuyerDTOToProto(input.Buyer)
	if err != nil {
		return nil, err
	}
	return &userpb.UpdateBuyerByUserIDRequest{
		Buyer: buyer,
	}, nil
}
func UpdBuyByUseIDResponseToOutput(res *userpb.UpdateBuyerByUserIDResponse) (*dto.UpdBuyByUseIDOutput, error) {
	return &dto.UpdBuyByUseIDOutput{
		Message: res.GetMessage(),
		Success: res.GetSuccess(),
	}, nil
}

func GetBuyByUseIDInputToRequest(input *dto.GetBuyByUseIDInput) (*userpb.GetBuyerByUserIDRequest, error) {
	return &userpb.GetBuyerByUserIDRequest{
		UserId: input.UserID,
	}, nil
}
func GetBuyByUseIDResponseToOutput(res *userpb.GetBuyerByUserIDResponse) (*dto.GetBuyByUseIDOutput, error) {
	buyer, err := BuyerProtoToDTO(res.GetBuyer())
	if err != nil {
		return nil, err
	}
	return &dto.GetBuyByUseIDOutput{
		Message: res.GetMessage(),
		Success: res.GetSuccess(),
		Buyer:   buyer,
	}, nil
}

func DelBuyByUseIDInputToRequest(input *dto.DelBuyByUseIDInput) (*userpb.DelBuyerByUserIDRequest, error) {
	return &userpb.DelBuyerByUserIDRequest{
		UserId: input.UserID,
	}, nil
}
func DelBuyByUseIDResponseToOutput(res *userpb.DelBuyerByUserIDResponse) (*dto.DelBuyByUseIDOutput, error) {
	return &dto.DelBuyByUseIDOutput{
		Message: res.GetMessage(),
		Success: res.GetSuccess(),
	}, nil
}

// Adapter for Seller

func CreSelInputToRequest(input *dto.CreateSellerInput) (*userpb.CreateSellerRequest, error) {
	seller, err := SellerDTOToProto(input.Seller)
	if err != nil {
		return nil, err
	}
	return &userpb.CreateSellerRequest{
		Seller: seller,
		UserId: input.UserID,
	}, nil
}
func CreSelResponseToOutput(res *userpb.CreateSellerResponse) (*dto.CreateSellerOutput, error) {
	return &dto.CreateSellerOutput{
		Message: res.GetMessage(),
		Success: res.GetSuccess(),
	}, nil
}

func UpdSelByIDInputToRequest(input *dto.UpdSelByIDInput) (*userpb.UpdateSellerByIDRequest, error) {
	seller, err := SellerDTOToProto(input.Seller)
	if err != nil {
		return nil, err
	}
	return &userpb.UpdateSellerByIDRequest{
		Seller: seller,
		UserID: input.UserID,
	}, nil
}
func UpdSelByIDResponseToOutput(res *userpb.UpdateSellerByIDResponse) (*dto.UpdSelByIDOutput, error) {
	return &dto.UpdSelByIDOutput{
		Message: res.GetMessage(),
		Success: res.GetSuccess(),
	}, nil
}

func GetSelByIDInputToRequest(input *dto.GetSelByIDInput) (*userpb.GetSellerByIDRequest, error) {
	return &userpb.GetSellerByIDRequest{
		UserId: input.UserID,
	}, nil
}
func GetSelByIDResponseToOutput(res *userpb.GetSellerByIDResponse) (*dto.GetSelByIDOutput, error) {
	seller, err := SellerProtoToDTO(res.GetSeller())
	if err != nil {
		return nil, err
	}
	return &dto.GetSelByIDOutput{
		Message: res.Message,
		Success: res.Success,
		Seller:  seller,
	}, nil
}

func DelSelByIDInputToRequest(input *dto.DelSelByIDInput) (*userpb.DelSellerByIDRequest, error) {
	return &userpb.DelSellerByIDRequest{
		UserId: input.UserID,
	}, nil
}
func DelSelByIDResponseToOutput(res *userpb.DelSellerByIDResponse) (*dto.DelSelByIDOutput, error) {
	return &dto.DelSelByIDOutput{
		Message: res.GetMessage(),
		Success: res.GetSuccess(),
	}, nil
}
