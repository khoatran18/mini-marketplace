// This file defines adapter functions that convert between
// gRPC response/request (generated from protobuf) from this server (ProductServer)
// and the internal domain dto used by service.

package adapter

import (
	"encoding/json"
	"product-service/pkg/dto"
	"product-service/pkg/pb"

	"google.golang.org/protobuf/types/known/structpb"
	"gorm.io/datatypes"
)

func StructPBToJSON(s *structpb.Struct) (datatypes.JSON, error) {
	if s == nil {
		return datatypes.JSON([]byte("{}")), nil
	}
	b, err := json.Marshal(s.AsMap())
	if err != nil {
		return datatypes.JSON([]byte("{}")), err
	}
	return datatypes.JSON(b), nil
}
func JSONToStructPB(j datatypes.JSON) (*structpb.Struct, error) {
	if len(j) == 0 || string(j) == "{}" {
		return structpb.NewStruct(map[string]interface{}{})
	}
	var m map[string]interface{}
	if err := json.Unmarshal(j, &m); err != nil {
		return nil, err
	}
	return structpb.NewStruct(m)
}

func ProductProtoToDTO(p *productpb.Product) (*dto.Product, error) {
	attributes, err := StructPBToJSON(p.GetAttributes())
	if err != nil {
		return nil, err
	}
	return &dto.Product{
		ID:         p.Id,
		Name:       p.Name,
		Price:      p.Price,
		SellerID:   p.SellerId,
		Inventory:  p.Inventory,
		Attributes: attributes,
	}, nil
}
func ProductDTOToProto(p *dto.Product) (*productpb.Product, error) {
	if p == nil {
		return &productpb.Product{}, nil
	}
	attributes, err := JSONToStructPB(p.Attributes)
	if err != nil {
		return nil, err
	}
	return &productpb.Product{
		Id:         p.ID,
		Name:       p.Name,
		Price:      p.Price,
		SellerId:   p.SellerID,
		Inventory:  p.Inventory,
		Attributes: attributes,
	}, nil
}

func ProductsDTOToProto(p []*dto.Product) ([]*productpb.Product, error) {
	var products []*productpb.Product
	for _, p := range p {
		product, err := ProductDTOToProto(p)
		if err != nil {
			return nil, err
		}
		products = append(products, product)
	}
	return products, nil
}

func CreProRequestToInput(req *productpb.CreateProductRequest) (*dto.CreateProductInput, error) {
	attributes, err := StructPBToJSON(req.GetAttributes())
	if err != nil {
		return nil, err
	}
	return &dto.CreateProductInput{
		Name:       req.GetName(),
		Price:      req.GetPrice(),
		SellerID:   req.GetSellerId(),
		Inventory:  req.GetInventory(),
		Attributes: attributes,
	}, nil
}
func CreProOutputToResponse(output *dto.CreateProductOutput) (*productpb.CreateProductResponse, error) {
	return &productpb.CreateProductResponse{
		Message: output.Message,
		Success: output.Success,
	}, nil
}

func UpdProRequestToInput(req *productpb.UpdateProductRequest) (*dto.UpdateProductInput, error) {
	product, err := ProductProtoToDTO(req.Product)
	if err != nil {
		return nil, err
	}
	return &dto.UpdateProductInput{
		UserID:  req.GetUserId(),
		Product: product,
	}, nil
}
func UpdProOutputToResponse(output *dto.UpdateProductOutput) (*productpb.UpdateProductResponse, error) {
	return &productpb.UpdateProductResponse{
		Message: output.Message,
		Success: output.Success,
	}, nil
}

func GetProByIDRequestToInput(req *productpb.GetProductByIDRequest) (*dto.GetProductByIDInput, error) {
	return &dto.GetProductByIDInput{
		SellerID: req.GetUserId(),
		ID:       req.GetId(),
	}, nil
}
func GetProByIDOutputToResponse(output *dto.GetProductByIDOutput) (*productpb.GetProductByIDResponse, error) {
	product, err := ProductDTOToProto(output.Product)
	if err != nil {
		return nil, err
	}
	return &productpb.GetProductByIDResponse{
		Message: output.Message,
		Success: output.Success,
		Product: product,
	}, nil
}

func GetProsBySelIDRequestToInput(req *productpb.GetProductsBySellerIDRequest) (*dto.GetProductsBySellerIDInput, error) {
	return &dto.GetProductsBySellerIDInput{
		SellerID: req.GetSellerId(),
	}, nil
}
func GetProsBySelIDOutputToResponse(output *dto.GetProductsBySellerIDOutput) (*productpb.GetProductsBySellerIDResponse, error) {
	products, err := ProductsDTOToProto(output.Products)
	if err != nil {
		return nil, err
	}
	return &productpb.GetProductsBySellerIDResponse{
		Message:  output.Message,
		Success:  output.Success,
		Products: products,
	}, nil
}

func GetInvByIDRequestToInput(req *productpb.GetInventoryByIDRequest) (*dto.GetInventoryByIDInput, error) {
	return &dto.GetInventoryByIDInput{
		ID:     req.GetId(),
		UserID: req.GetUserId(),
	}, nil
}
func GetInvByIDOutputToResponse(output *dto.GetInventoryByIDOutput) (*productpb.GetInventoryByIDResponse, error) {
	return &productpb.GetInventoryByIDResponse{
		Message:   output.Message,
		Success:   output.Success,
		Inventory: output.Inventory,
	}, nil
}

func GetAndDecInvByIDRequestToInput(req *productpb.GetAndDecreaseInventoryByIDRequest) (*dto.GetAndDecreaseInventoryByIDInput, error) {
	return &dto.GetAndDecreaseInventoryByIDInput{
		ID:       req.GetId(),
		Quantity: req.GetQuantity(),
		UserID:   req.GetUserId(),
	}, nil
}

func GetAndDecInvByIDOutputToResponse(output *dto.GetAndDecreaseInventoryByIDOutput) (*productpb.GetAndDecreaseInventoryByIDResponse, error) {
	return &productpb.GetAndDecreaseInventoryByIDResponse{
		Message: output.Message,
		Success: output.Success,
	}, nil
}
