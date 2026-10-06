package productclient

import (
	"api-gateway/pkg/dto"
	productpb "api-gateway/pkg/pb/productservice"

	"google.golang.org/protobuf/types/known/structpb"
)

func MapToStruct(m map[string]any) (*structpb.Struct, error) {
	return structpb.NewStruct(m)
}
func StructToMap(s *structpb.Struct) (map[string]any, error) {
	return s.AsMap(), nil
}

func ProductDTOToProto(product *dto.Product) (*productpb.Product, error) {
	if product == nil {
		return nil, nil
	}
	attributes, err := MapToStruct(product.Attributes)
	if err != nil {
		return nil, err
	}
	return &productpb.Product{
		Id:         product.ID,
		Name:       product.Name,
		Price:      product.Price,
		SellerId:   product.SellerID,
		Inventory:  product.Inventory,
		Attributes: attributes,
	}, nil
}
func ProductProtoToDTO(product *productpb.Product) (*dto.Product, error) {
	if product == nil {
		return nil, nil
	}
	attributes, err := StructToMap(product.Attributes)
	if err != nil {
		return nil, err
	}
	return &dto.Product{
		ID:         product.GetId(),
		Name:       product.GetName(),
		Price:      product.GetPrice(),
		SellerID:   product.GetSellerId(),
		Inventory:  product.GetInventory(),
		Attributes: attributes,
	}, nil
}

func ProductsDTOToProto(products []*productpb.Product) ([]*dto.Product, error) {
	var productsDTO []*dto.Product
	for _, product := range products {
		productDTO, err := ProductProtoToDTO(product)
		if err != nil {
			return nil, err
		}
		productsDTO = append(productsDTO, productDTO)
	}
	return productsDTO, nil
}
func ProductsProtoToDTO(products []*productpb.Product) ([]*dto.Product, error) {
	var productsDTO []*dto.Product
	for _, product := range products {
		productDTO, err := ProductProtoToDTO(product)
		if err != nil {
			return nil, err
		}
		productsDTO = append(productsDTO, productDTO)
	}
	return productsDTO, nil
}

func CreateProductInputToRequest(input *dto.CreateProductInput) (*productpb.CreateProductRequest, error) {
	attributes, err := MapToStruct(input.Attributes)
	if err != nil {
		return nil, err
	}
	return &productpb.CreateProductRequest{
		Name:       input.Name,
		Price:      input.Price,
		SellerId:   input.SellerID,
		Inventory:  input.Inventory,
		Attributes: attributes,
	}, nil
}
func CreateProductResponseToOutput(res *productpb.CreateProductResponse) (*dto.CreateProductOutput, error) {
	return &dto.CreateProductOutput{
		Message: res.GetMessage(),
		Success: res.GetSuccess(),
	}, nil
}

func UpdateProductInputToRequest(input *dto.UpdateProductInput) (*productpb.UpdateProductRequest, error) {
	product, err := ProductDTOToProto(input.Product)
	if err != nil {
		return nil, err
	}
	return &productpb.UpdateProductRequest{
		Product: product,
		UserId:  input.UserId,
	}, nil
}
func UpdateProductResponseToOutput(res *productpb.UpdateProductResponse) (*dto.UpdateProductOutput, error) {
	return &dto.UpdateProductOutput{
		Message: res.GetMessage(),
		Success: res.GetSuccess(),
	}, nil
}

func GetProductByIDInputToRequest(input *dto.GetProductByIDInput) (*productpb.GetProductByIDRequest, error) {
	return &productpb.GetProductByIDRequest{
		Id: input.ProductID,
	}, nil
}
func GetProductByIDResponseToOutput(res *productpb.GetProductByIDResponse) (*dto.GetProductByIDOutput, error) {
	product, err := ProductProtoToDTO(res.GetProduct())
	if err != nil {
		return nil, err
	}
	return &dto.GetProductByIDOutput{
		Message: res.GetMessage(),
		Success: res.GetSuccess(),
		Product: product,
	}, nil
}

func GetProductsBySellerIDInputToRequest(input *dto.GetProductsBySellerIDInput) (*productpb.GetProductsBySellerIDRequest, error) {
	return &productpb.GetProductsBySellerIDRequest{
		SellerId: input.SellerID,
	}, nil
}
func GetProductsBySellerIDResponseToOutput(res *productpb.GetProductsBySellerIDResponse) (*dto.GetProductsBySellerIDOutput, error) {
	products, err := ProductsDTOToProto(res.GetProducts())
	if err != nil {
		return nil, err
	}
	return &dto.GetProductsBySellerIDOutput{
		Message:  res.GetMessage(),
		Success:  res.GetSuccess(),
		Products: products,
	}, nil
}

func GetProductsInputToRequest(input *dto.GetProductsInput) (*productpb.GetProductsRequest, error) {
	return &productpb.GetProductsRequest{
		Page:     input.Page,
		PageSize: input.PageSize,
	}, nil
}
func GetProductsResponseToOutput(res *productpb.GetProductsResponse) (*dto.GetProductsOutput, error) {
	products, err := ProductsProtoToDTO(res.GetProduct())
	if err != nil {
		return nil, err
	}
	return &dto.GetProductsOutput{
		Message:  res.GetMessage(),
		Success:  res.GetSuccess(),
		Products: products,
	}, nil
}
