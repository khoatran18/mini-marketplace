package mapper

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

func ProductProtoToDTO(p *pb.Product) (*dto.Product, error) {
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
func ProductDTOToProto(p *dto.Product) (*pb.Product, error) {
	if p == nil {
		return &pb.Product{}, nil
	}
	attributes, err := JSONToStructPB(p.Attributes)
	if err != nil {
		return nil, err
	}
	return &pb.Product{
		Id:         p.ID,
		Name:       p.Name,
		Price:      p.Price,
		SellerId:   p.SellerID,
		Inventory:  p.Inventory,
		Attributes: attributes,
	}, nil
}

func ProductsDTOToProto(p []*dto.Product) ([]*pb.Product, error) {
	var products []*pb.Product
	for _, p := range p {
		product, err := ProductDTOToProto(p)
		if err != nil {
			return nil, err
		}
		products = append(products, product)
	}
	return products, nil
}

func CreProRequestToInput(req *pb.CreateProductRequest) (*dto.CreateProductInput, error) {
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
func CreProOutputToResponse(output *dto.CreateProductOutput) (*pb.CreateProductResponse, error) {
	return &pb.CreateProductResponse{
		Message: output.Message,
		Success: output.Success,
	}, nil
}

func UpdProRequestToInput(req *pb.UpdateProductRequest) (*dto.UpdateProductInput, error) {
	product, err := ProductProtoToDTO(req.Product)
	if err != nil {
		return nil, err
	}
	return &dto.UpdateProductInput{
		UserID:  req.GetUserId(),
		Product: product,
	}, nil
}
func UpdProOutputToResponse(output *dto.UpdateProductOutput) (*pb.UpdateProductResponse, error) {
	return &pb.UpdateProductResponse{
		Message: output.Message,
		Success: output.Success,
	}, nil
}

func GetProByIDRequestToInput(req *pb.GetProductByIDRequest) (*dto.GetProductByIDInput, error) {
	return &dto.GetProductByIDInput{
		SellerID: req.GetUserId(),
		ID:       req.GetId(),
	}, nil
}
func GetProByIDOutputToResponse(output *dto.GetProductByIDOutput) (*pb.GetProductByIDResponse, error) {
	product, err := ProductDTOToProto(output.Product)
	if err != nil {
		return nil, err
	}
	return &pb.GetProductByIDResponse{
		Message: output.Message,
		Success: output.Success,
		Product: product,
	}, nil
}

func GetProsBySelIDRequestToInput(req *pb.GetProductsBySellerIDRequest) (*dto.GetProductsBySellerIDInput, error) {
	return &dto.GetProductsBySellerIDInput{
		SellerID: req.GetSellerId(),
	}, nil
}
func GetProsBySelIDOutputToResponse(output *dto.GetProductsBySellerIDOutput) (*pb.GetProductsBySellerIDResponse, error) {
	products, err := ProductsDTOToProto(output.Products)
	if err != nil {
		return nil, err
	}
	return &pb.GetProductsBySellerIDResponse{
		Message:  output.Message,
		Success:  output.Success,
		Products: products,
	}, nil
}

func GetInvByIDRequestToInput(req *pb.GetInventoryByIDRequest) (*dto.GetInventoryByIDInput, error) {
	return &dto.GetInventoryByIDInput{
		ID:     req.GetId(),
		UserID: req.GetUserId(),
	}, nil
}
func GetInvByIDOutputToResponse(output *dto.GetInventoryByIDOutput) (*pb.GetInventoryByIDResponse, error) {
	return &pb.GetInventoryByIDResponse{
		Message:   output.Message,
		Success:   output.Success,
		Inventory: output.Inventory,
	}, nil
}

func GetAndDecInvByIDRequestToInput(req *pb.GetAndDecreaseInventoryByIDRequest) (*dto.GetAndDecreaseInventoryByIDInput, error) {
	return &dto.GetAndDecreaseInventoryByIDInput{
		ID:       req.GetId(),
		Quantity: req.GetQuantity(),
		UserID:   req.GetUserId(),
	}, nil
}

func GetAndDecInvByIDOutputToResponse(output *dto.GetAndDecreaseInventoryByIDOutput) (*pb.GetAndDecreaseInventoryByIDResponse, error) {
	return &pb.GetAndDecreaseInventoryByIDResponse{
		Message: output.Message,
		Success: output.Success,
	}, nil
}
