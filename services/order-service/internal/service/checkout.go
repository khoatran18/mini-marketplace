package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"order-service/internal/client/productclient"
	"order-service/internal/repository"
	"order-service/pkg/model"
	"regexp"
	"slices"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Line is a requested product and quantity.
type Line struct {
	ProductID uint64
	Quantity  int64
}

// PreviewLine is a priced line, possibly with an issue that blocks ordering it.
type PreviewLine struct {
	ProductID  uint64
	Name       string
	ImageURL   string
	Quantity   int64
	UnitPrice  float64
	LineTotal  float64
	StockLevel string
	Available  bool
	Issue      string
}

// PreviewGroup is the part of a checkout that belongs to one store (it becomes one order).
type PreviewGroup struct {
	StoreID     uint64
	Lines       []*PreviewLine
	Subtotal    float64
	ShippingFee float64
	Total       float64

	category map[uint64]uint64
	products map[uint64]*productclient.ProductDTOClient
}

// Preview is the server-side price of a checkout. Nothing the client sends about prices is ever used.
type Preview struct {
	Groups      []*PreviewGroup
	Subtotal    float64
	ShippingFee float64
	GrandTotal  float64
	CanOrder    bool
}

// mergeLines merges duplicates and validates quantities.
func mergeLines(lines []Line) ([]Line, error) {
	if len(lines) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one item is required")
	}
	qty := map[uint64]int64{}
	for _, l := range lines {
		if l.ProductID == 0 {
			return nil, status.Error(codes.InvalidArgument, "item product_id is required")
		}
		if l.Quantity <= 0 || l.Quantity > maxItemQuantity {
			return nil, status.Errorf(codes.InvalidArgument, "quantity of product %d must be between 1 and %d", l.ProductID, maxItemQuantity)
		}
		qty[l.ProductID] += l.Quantity
		if qty[l.ProductID] > maxItemQuantity {
			return nil, status.Errorf(codes.InvalidArgument, "quantity of product %d must be between 1 and %d", l.ProductID, maxItemQuantity)
		}
	}
	if len(qty) > maxCheckoutLines {
		return nil, status.Errorf(codes.InvalidArgument, "at most %d different products per checkout", maxCheckoutLines)
	}
	out := make([]Line, 0, len(qty))
	for id, q := range qty {
		out = append(out, Line{ProductID: id, Quantity: q})
	}
	slices.SortFunc(out, func(a, b Line) int { return int(int64(a.ProductID) - int64(b.ProductID)) })
	return out, nil
}

// linesFor returns the lines to price: the cart of the buyer or the explicit items.
func (s *OrderService) linesFor(ctx context.Context, buyerID uint64, fromCart bool, items []Line) ([]Line, error) {
	if buyerID == 0 {
		return nil, status.Error(codes.InvalidArgument, "buyer_id is required")
	}
	if !fromCart {
		return mergeLines(items)
	}
	cart, err := s.OrderRepo.GetCartItems(ctx, buyerID)
	if err != nil {
		return nil, err
	}
	if len(cart) == 0 {
		return nil, status.Error(codes.InvalidArgument, "the cart is empty")
	}
	lines := make([]Line, 0, len(cart))
	for _, c := range cart {
		lines = append(lines, Line{ProductID: c.ProductID, Quantity: c.Quantity})
	}
	return mergeLines(lines)
}

// lineIssue explains why a line can not be ordered ("" when it can).
func lineIssue(p *productclient.ProductDTOClient, qty int64) string {
	switch {
	case p == nil:
		return "product not found"
	case p.Status != productActive:
		return "product is not available"
	case toMinor(p.Price) <= 0:
		return "product is not available for sale"
	case p.Inventory <= 0:
		return "out of stock"
	case p.Inventory < qty:
		return fmt.Sprintf("only %d left", p.Inventory)
	}
	return ""
}

// price builds the preview of lines: grouped per store, priced from the catalog, shipping added per store.
func (s *OrderService) price(ctx context.Context, lines []Line) (*Preview, error) {
	ids := make([]uint64, 0, len(lines))
	for _, l := range lines {
		ids = append(ids, l.ProductID)
	}
	catalog, err := s.loadCatalog(ctx, ids)
	if err != nil {
		return nil, err
	}
	groups := map[uint64]*PreviewGroup{}
	var storeIDs []uint64
	pv := &Preview{CanOrder: true}
	for _, l := range lines {
		p := catalog[l.ProductID]
		issue := lineIssue(p, l.Quantity)
		pl := &PreviewLine{ProductID: l.ProductID, Quantity: l.Quantity, Issue: issue, Available: issue == ""}
		store := uint64(0)
		if p != nil {
			pl.Name, pl.ImageURL, pl.UnitPrice, pl.StockLevel = p.Name, p.ImageURL, p.Price, p.StockLevel
			pl.LineTotal = fromMinor(toMinor(p.Price) * l.Quantity)
			store = p.SellerID
		}
		if issue != "" {
			pv.CanOrder = false
		}
		g := groups[store]
		if g == nil {
			g = &PreviewGroup{StoreID: store, category: map[uint64]uint64{}, products: map[uint64]*productclient.ProductDTOClient{}}
			groups[store] = g
			storeIDs = append(storeIDs, store)
		}
		g.Lines = append(g.Lines, pl)
		if p != nil {
			g.products[l.ProductID] = p
		}
	}
	slices.Sort(storeIDs)
	var subMinor, shipMinor int64
	for _, id := range storeIDs {
		g := groups[id]
		var gMinor int64
		for _, l := range g.Lines {
			if l.Available {
				gMinor += toMinor(l.UnitPrice) * l.Quantity
			}
		}
		fee := s.shippingFeeMinor(gMinor)
		if gMinor == 0 {
			fee = 0
		}
		g.Subtotal, g.ShippingFee, g.Total = fromMinor(gMinor), fromMinor(fee), fromMinor(gMinor+fee)
		subMinor += gMinor
		shipMinor += fee
		pv.Groups = append(pv.Groups, g)
	}
	pv.Subtotal, pv.ShippingFee, pv.GrandTotal = fromMinor(subMinor), fromMinor(shipMinor), fromMinor(subMinor+shipMinor)
	return pv, nil
}

// PreviewCheckout prices a checkout without creating anything.
func (s *OrderService) PreviewCheckout(ctx context.Context, buyerID uint64, fromCart bool, items []Line) (*Preview, error) {
	lines, err := s.linesFor(ctx, buyerID, fromCart, items)
	if err != nil {
		return nil, err
	}
	return s.price(ctx, lines)
}

// CheckoutInput is a request to place orders.
type CheckoutInput struct {
	BuyerID        uint64
	FromCart       bool
	Items          []Line
	PaymentMethod  string
	Address        map[string]any
	Note           string
	IdempotencyKey string
}

// CheckoutResult is what Checkout returns.
type CheckoutResult struct {
	Checkout *model.Checkout
	Orders   []*model.Order
	Replayed bool
}

var idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_\-]{1,64}$`)

// validateAddress requires the fields a carrier needs and returns the JSON snapshot to store.
func validateAddress(addr map[string]any) (datatypes.JSON, error) {
	need := func(key string, max int) (string, error) {
		v, _ := addr[key].(string)
		v = strings.TrimSpace(v)
		if v == "" || len([]rune(v)) > max {
			return "", status.Errorf(codes.InvalidArgument, "shipping address field %q is required (max %d characters)", key, max)
		}
		return v, nil
	}
	clean := map[string]any{}
	for key, max := range map[string]int{"receiver_name": 100, "phone": 20, "line1": 200, "city": 100} {
		v, err := need(key, max)
		if err != nil {
			return nil, err
		}
		clean[key] = v
	}
	for _, key := range []string{"ward", "district", "label"} {
		if v, ok := addr[key].(string); ok && len([]rune(v)) <= 100 {
			clean[key] = strings.TrimSpace(v)
		}
	}
	b, err := json.Marshal(clean)
	return datatypes.JSON(b), err
}

func newCheckoutID() (string, error) {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "co_" + hex.EncodeToString(b), nil
}

// Checkout turns the cart (or explicit items) into one order per store. It is idempotent per
// (buyer, idempotency key): repeating the call returns the same checkout without creating anything.
func (s *OrderService) Checkout(ctx context.Context, in *CheckoutInput) (*CheckoutResult, error) {
	if in == nil || in.BuyerID == 0 {
		return nil, status.Error(codes.InvalidArgument, "buyer_id is required")
	}
	if !idempotencyKeyPattern.MatchString(in.IdempotencyKey) {
		return nil, status.Error(codes.InvalidArgument, "idempotency key must be 1-64 characters of letters, digits, '-' or '_'")
	}
	if !model.ValidMethod(in.PaymentMethod) {
		return nil, status.Error(codes.InvalidArgument, "unsupported payment method")
	}
	if len([]rune(in.Note)) > 500 {
		return nil, status.Error(codes.InvalidArgument, "note must be at most 500 characters")
	}
	address, err := validateAddress(in.Address)
	if err != nil {
		return nil, err
	}

	// A repeated request returns the original result before anything is priced again
	if c, orders, err := s.OrderRepo.CheckoutByKey(ctx, in.BuyerID, in.IdempotencyKey); err == nil {
		return &CheckoutResult{Checkout: c, Orders: orders, Replayed: true}, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	lines, err := s.linesFor(ctx, in.BuyerID, in.FromCart, in.Items)
	if err != nil {
		return nil, err
	}
	pv, err := s.price(ctx, lines)
	if err != nil {
		return nil, err
	}
	for _, g := range pv.Groups {
		for _, l := range g.Lines {
			if l.Issue != "" {
				name := l.Name
				if name == "" {
					name = fmt.Sprintf("product %d", l.ProductID)
				}
				return nil, status.Errorf(codes.FailedPrecondition, "%s: %s", name, l.Issue)
			}
		}
	}

	checkoutID, err := newCheckoutID()
	if err != nil {
		return nil, err
	}
	create := &repository.CheckoutCreate{
		Checkout:   &model.Checkout{ID: checkoutID, BuyerID: in.BuyerID, IdempotencyKey: in.IdempotencyKey, PaymentMethod: in.PaymentMethod, Total: pv.GrandTotal},
		ActorBuyer: in.BuyerID,
	}
	for _, g := range pv.Groups {
		o := &model.Order{
			BuyerID: in.BuyerID, StoreID: g.StoreID, Status: model.StatusPending, PaymentMethod: in.PaymentMethod,
			PaymentStatus: model.PaymentUnpaid, Subtotal: g.Subtotal, ShippingFee: g.ShippingFee, TotalPrice: g.Total,
			ShippingAddress: address, Note: in.Note,
		}
		for _, l := range g.Lines {
			p := g.products[l.ProductID]
			o.OrderItems = append(o.OrderItems, &model.OrderItem{
				ProductID: l.ProductID, Quantity: l.Quantity, Price: l.UnitPrice, LineTotal: l.LineTotal, Status: "ACTIVE",
				ProductName: p.Name, SKU: p.SKU, ImageURL: p.ImageURL, StoreID: g.StoreID, CategoryID: p.CategoryID,
			})
		}
		create.Orders = append(create.Orders, o)
	}
	if in.FromCart {
		create.CartClear = in.BuyerID
		for _, l := range lines {
			create.CartLines = append(create.CartLines, l.ProductID)
		}
	}
	c, orders, replayed, err := s.OrderRepo.CreateCheckout(ctx, create)
	if err != nil {
		s.ZapLogger.Error("OrderService: create checkout failed")
		return nil, status.Error(codes.Internal, "can not create the order")
	}
	return &CheckoutResult{Checkout: c, Orders: orders, Replayed: replayed}, nil
}

// GetCheckout returns a buyer's checkout.
func (s *OrderService) GetCheckout(ctx context.Context, buyerID uint64, id string) (*CheckoutResult, error) {
	c, orders, err := s.OrderRepo.GetCheckout(ctx, buyerID, id)
	if err != nil {
		return nil, orderError(err)
	}
	return &CheckoutResult{Checkout: c, Orders: orders}, nil
}
