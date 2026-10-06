package service

import (
	"context"
	"errors"
	"order-service/internal/repository"
	"order-service/pkg/model"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CartLine is a stored cart line enriched with live catalog data.
type CartLine struct {
	ProductID  uint64
	Quantity   int64
	Name       string
	ImageURL   string
	StoreID    uint64
	UnitPrice  float64
	LineTotal  float64
	StockLevel string
	Available  bool
	Issue      string
}

// Cart is the buyer's cart.
type Cart struct {
	Lines     []*CartLine
	Subtotal  float64
	ItemCount int64
}

// GetCart returns the cart with prices and stock from the catalog (the stored lines only hold product and quantity).
func (s *OrderService) GetCart(ctx context.Context, userID uint64) (*Cart, error) {
	if userID == 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	items, err := s.OrderRepo.GetCartItems(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids := make([]uint64, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ProductID)
	}
	catalog, err := s.loadCatalog(ctx, ids)
	if err != nil {
		return nil, err
	}
	cart := &Cart{Lines: []*CartLine{}}
	var subMinor int64
	for _, it := range items {
		p := catalog[it.ProductID]
		issue := lineIssue(p, it.Quantity)
		l := &CartLine{ProductID: it.ProductID, Quantity: it.Quantity, Issue: issue, Available: issue == ""}
		if p != nil {
			l.Name, l.ImageURL, l.StoreID, l.UnitPrice, l.StockLevel = p.Name, p.ImageURL, p.SellerID, p.Price, p.StockLevel
			l.LineTotal = fromMinor(toMinor(p.Price) * it.Quantity)
		}
		if l.Available {
			subMinor += toMinor(p.Price) * it.Quantity
		}
		cart.ItemCount += it.Quantity
		cart.Lines = append(cart.Lines, l)
	}
	cart.Subtotal = fromMinor(subMinor)
	return cart, nil
}

func cartError(err error) error {
	if errors.Is(err, repository.ErrCartFull) {
		return status.Errorf(codes.FailedPrecondition, "the cart can hold at most %d different products", model.MaxCartLines)
	}
	return err
}

// SetCartItem sets the quantity of a product (0 removes it) and returns the cart. Adding checks that the
// product exists, is active and has enough stock.
func (s *OrderService) SetCartItem(ctx context.Context, userID, productID uint64, quantity int64) (*Cart, error) {
	if userID == 0 || productID == 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id and product_id are required")
	}
	if quantity < 0 || quantity > repository.MaxCartQuantity {
		return nil, status.Errorf(codes.InvalidArgument, "quantity must be between 0 and %d", repository.MaxCartQuantity)
	}
	if quantity > 0 {
		catalog, err := s.loadCatalog(ctx, []uint64{productID})
		if err != nil {
			return nil, err
		}
		if issue := lineIssue(catalog[productID], quantity); issue != "" {
			return nil, status.Error(codes.FailedPrecondition, issue)
		}
	}
	if err := s.OrderRepo.SetCartItem(ctx, userID, productID, quantity); err != nil {
		return nil, cartError(err)
	}
	return s.GetCart(ctx, userID)
}

// ClearCart empties the cart.
func (s *OrderService) ClearCart(ctx context.Context, userID uint64) (*Cart, error) {
	if userID == 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if err := s.OrderRepo.ClearCart(ctx, userID); err != nil {
		return nil, err
	}
	return s.GetCart(ctx, userID)
}

// MergeCart adds a guest cart (localStorage) to the stored cart after login. Unknown or unavailable products are
// skipped; quantities are capped; the result is returned.
func (s *OrderService) MergeCart(ctx context.Context, userID uint64, items []Line) (*Cart, error) {
	if userID == 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if len(items) > model.MaxCartLines {
		items = items[:model.MaxCartLines]
	}
	ids := make([]uint64, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ProductID)
	}
	catalog, err := s.loadCatalog(ctx, ids)
	if err != nil {
		return nil, err
	}
	lines := map[uint64]int64{}
	for _, it := range items {
		if p := catalog[it.ProductID]; p != nil && p.Status == productActive && it.Quantity > 0 {
			lines[it.ProductID] += it.Quantity
		}
	}
	if err := s.OrderRepo.MergeCart(ctx, userID, lines); err != nil {
		return nil, cartError(err)
	}
	return s.GetCart(ctx, userID)
}
