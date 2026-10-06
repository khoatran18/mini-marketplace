package service

import (
	"context"
	"regexp"
	"strings"
	"unicode"

	"product-service/internal/repository"
	"product-service/internal/service/adapter"
	"product-service/pkg/dto"
	"product-service/pkg/model"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SearchProducts runs a keyword search with filters (see repository.SearchParams).
func (s *ProductService) SearchProducts(ctx context.Context, p repository.SearchParams) ([]*dto.Product, int64, error) {
	if len([]rune(p.Query)) > 200 {
		return nil, 0, status.Error(codes.InvalidArgument, "query is too long")
	}
	switch p.Sort {
	case "", repository.SortRelevance, repository.SortPriceAsc, repository.SortPriceDesc, repository.SortNewest, repository.SortBestSelling:
	default:
		return nil, 0, status.Error(codes.InvalidArgument, "unknown sort")
	}
	if p.PriceMax > 0 && p.PriceMin > p.PriceMax {
		return nil, 0, status.Error(codes.InvalidArgument, "price_min must not be greater than price_max")
	}
	products, total, err := s.ProductRepo.SearchProducts(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	return adapter.ProductsModelToDTO(products), total, nil
}

// CategoryView is a category with its (descendant-inclusive) number of active products.
type CategoryView struct {
	model.Category
	ProductCount int64
}

// ListCategories returns the flat category list (clients build the tree from ParentID).
func (s *ProductService) ListCategories(ctx context.Context, includeInactive bool) ([]*CategoryView, error) {
	cats, counts, err := s.ProductRepo.ListCategories(ctx, includeInactive)
	if err != nil {
		return nil, err
	}
	out := make([]*CategoryView, 0, len(cats))
	for _, c := range cats {
		out = append(out, &CategoryView{Category: *c, ProductCount: counts[c.ID]})
	}
	return out, nil
}

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// slugify lower-cases, removes Vietnamese diacritics and joins words with "-".
func slugify(name string) string {
	var b strings.Builder
	prevDash := true
	for _, r := range strings.ToLower(name) {
		switch {
		case r == 'đ':
			r = 'd'
		case r > unicode.MaxASCII:
			r = foldRune(r)
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func foldRune(r rune) rune {
	for base, letters := range map[rune]string{'a': "àáạảãâầấậẩẫăằắặẳẵ", 'e': "èéẹẻẽêềếệểễ", 'i': "ìíịỉĩ", 'o': "òóọỏõôồốộổỗơờớợởỡ", 'u': "ùúụủũưừứựửữ", 'y': "ỳýỵỷỹ"} {
		if strings.ContainsRune(letters, r) {
			return base
		}
	}
	return '-'
}

// UpsertCategory creates (ID 0) or updates a category (admin only; enforced by the gateway).
func (s *ProductService) UpsertCategory(ctx context.Context, c *model.Category) (*model.Category, error) {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" || len([]rune(c.Name)) > 100 {
		return nil, status.Error(codes.InvalidArgument, "name must be 1-100 characters")
	}
	c.Slug = strings.TrimSpace(c.Slug)
	if c.Slug == "" {
		c.Slug = slugify(c.Name)
	}
	if !slugPattern.MatchString(c.Slug) || len(c.Slug) > 100 {
		return nil, status.Error(codes.InvalidArgument, "slug must be lowercase letters, digits and dashes")
	}
	if err := s.ProductRepo.UpsertCategory(ctx, c); err != nil {
		if err.Error() == "categories can have at most 3 levels" {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, mapRepoError(err)
	}
	return c, nil
}

// SetProductStatus changes the visibility of a product (owner store or admin).
func (s *ProductService) SetProductStatus(ctx context.Context, productID, storeID uint64, isAdmin bool, newStatus string) error {
	if !validStatuses[newStatus] {
		return status.Error(codes.InvalidArgument, "status must be one of draft, active, hidden, banned")
	}
	if newStatus == model.StatusBanned && !isAdmin {
		return status.Error(codes.PermissionDenied, "only an administrator can ban a product")
	}
	return mapRepoError(s.ProductRepo.SetProductStatus(ctx, productID, storeID, isAdmin, newStatus))
}

// AdjustInventory applies a manual stock correction (owner store only) and returns the new available stock.
func (s *ProductService) AdjustInventory(ctx context.Context, productID, storeID uint64, delta int64, reason string) (int64, error) {
	reason = strings.TrimSpace(reason)
	if delta == 0 {
		return 0, status.Error(codes.InvalidArgument, "delta must not be zero")
	}
	if delta > 1_000_000 || delta < -1_000_000 {
		return 0, status.Error(codes.InvalidArgument, "delta is out of range")
	}
	if reason == "" || len([]rune(reason)) > 200 {
		return 0, status.Error(codes.InvalidArgument, "a reason (1-200 characters) is required")
	}
	inv, err := s.ProductRepo.AdjustInventory(ctx, productID, storeID, delta, reason)
	return inv, mapRepoError(err)
}

// GetInventoryLedger returns the stock history of a product owned by storeID.
func (s *ProductService) GetInventoryLedger(ctx context.Context, productID, storeID, page, pageSize uint64) ([]*model.InventoryLedger, error) {
	entries, err := s.ProductRepo.GetLedger(ctx, productID, storeID, page, pageSize)
	return entries, mapRepoError(err)
}

// ListLowStock lists a store's products at or below their low-stock threshold.
func (s *ProductService) ListLowStock(ctx context.Context, storeID uint64, defaultThreshold int64) ([]*dto.Product, error) {
	if storeID == 0 {
		return nil, status.Error(codes.InvalidArgument, "store_id is required")
	}
	products, err := s.ProductRepo.ListLowStock(ctx, storeID, defaultThreshold)
	if err != nil {
		return nil, err
	}
	return adapter.ProductsModelToDTO(products), nil
}
