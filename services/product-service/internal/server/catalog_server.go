package server

import (
	"context"
	"product-service/internal/repository"
	"product-service/internal/server/adapter"
	"product-service/internal/service"
	"product-service/pkg/model"
	"product-service/pkg/pb"
	"time"
)

func categoryToProto(c *service.CategoryView) *productpb.Category {
	return &productpb.Category{Id: c.ID, ParentId: c.ParentID, Name: c.Name, Slug: c.Slug, Sort: int32(c.Sort), Active: c.Active, ProductCount: c.ProductCount}
}

// SearchProducts handles the keyword search RPC.
func (s *ProductServer) SearchProducts(ctx context.Context, req *productpb.SearchProductsRequest) (*productpb.SearchProductsResponse, error) {
	products, total, err := s.ProductService.SearchProducts(ctx, repository.SearchParams{
		Query: req.GetQuery(), CategoryID: req.GetCategoryId(), PriceMin: req.GetPriceMin(), PriceMax: req.GetPriceMax(),
		Brand: req.GetBrand(), SellerID: req.GetSellerId(), InStockOnly: req.GetInStockOnly(), Sort: req.GetSort(),
		Page: req.GetPage(), PageSize: req.GetPageSize(), IncludeAllStatuses: req.GetIncludeAllStatuses(),
	})
	if err != nil {
		return nil, err
	}
	out, err := adapter.ProductsDTOToProto(products)
	if err != nil {
		return nil, err
	}
	return &productpb.SearchProductsResponse{Message: "ok", Success: true, Products: out, Total: uint64(total)}, nil
}

// ListCategories returns the flat category list.
func (s *ProductServer) ListCategories(ctx context.Context, req *productpb.ListCategoriesRequest) (*productpb.ListCategoriesResponse, error) {
	cats, err := s.ProductService.ListCategories(ctx, req.GetIncludeInactive())
	if err != nil {
		return nil, err
	}
	out := make([]*productpb.Category, 0, len(cats))
	for _, c := range cats {
		out = append(out, categoryToProto(c))
	}
	return &productpb.ListCategoriesResponse{Message: "ok", Success: true, Categories: out}, nil
}

// UpsertCategory creates or updates a category.
func (s *ProductServer) UpsertCategory(ctx context.Context, req *productpb.UpsertCategoryRequest) (*productpb.UpsertCategoryResponse, error) {
	in := req.GetCategory()
	c, err := s.ProductService.UpsertCategory(ctx, &model.Category{
		ID: in.GetId(), ParentID: in.GetParentId(), Name: in.GetName(), Slug: in.GetSlug(), Sort: int(in.GetSort()), Active: in.GetActive(),
	})
	if err != nil {
		return nil, err
	}
	return &productpb.UpsertCategoryResponse{Message: "ok", Success: true, Category: categoryToProto(&service.CategoryView{Category: *c})}, nil
}

// SetProductStatus changes a product's visibility.
func (s *ProductServer) SetProductStatus(ctx context.Context, req *productpb.SetProductStatusRequest) (*productpb.SetProductStatusResponse, error) {
	if err := s.ProductService.SetProductStatus(ctx, req.GetProductId(), req.GetStoreId(), req.GetIsAdmin(), req.GetStatus()); err != nil {
		return nil, err
	}
	return &productpb.SetProductStatusResponse{Message: "ok", Success: true}, nil
}

// AdjustInventory applies a manual stock correction.
func (s *ProductServer) AdjustInventory(ctx context.Context, req *productpb.AdjustInventoryRequest) (*productpb.AdjustInventoryResponse, error) {
	inv, err := s.ProductService.AdjustInventory(ctx, req.GetProductId(), req.GetStoreId(), req.GetDelta(), req.GetReason())
	if err != nil {
		return nil, err
	}
	return &productpb.AdjustInventoryResponse{Message: "ok", Success: true, Inventory: inv}, nil
}

// GetInventoryLedger returns the stock history of a product.
func (s *ProductServer) GetInventoryLedger(ctx context.Context, req *productpb.GetInventoryLedgerRequest) (*productpb.GetInventoryLedgerResponse, error) {
	entries, err := s.ProductService.GetInventoryLedger(ctx, req.GetProductId(), req.GetStoreId(), req.GetPage(), req.GetPageSize())
	if err != nil {
		return nil, err
	}
	out := make([]*productpb.LedgerEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, &productpb.LedgerEntry{
			Id: e.ID, ProductId: e.ProductID, Delta: e.Delta, Reason: e.Reason, RefType: e.RefType, RefId: e.RefID,
			BalanceAfter: e.BalanceAfter, At: e.At.UTC().Format(time.RFC3339),
		})
	}
	return &productpb.GetInventoryLedgerResponse{Message: "ok", Success: true, Entries: out}, nil
}

// ListLowStock lists a store's low-stock products.
func (s *ProductServer) ListLowStock(ctx context.Context, req *productpb.ListLowStockRequest) (*productpb.ListLowStockResponse, error) {
	products, err := s.ProductService.ListLowStock(ctx, req.GetStoreId(), req.GetDefaultThreshold())
	if err != nil {
		return nil, err
	}
	out, err := adapter.ProductsDTOToProto(products)
	if err != nil {
		return nil, err
	}
	return &productpb.ListLowStockResponse{Message: "ok", Success: true, Products: out}, nil
}
