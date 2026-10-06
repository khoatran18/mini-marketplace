package server

import (
	"context"
	"encoding/json"
	"order-service/internal/repository"
	"order-service/internal/service"
	"order-service/pkg/model"
	orderpb "order-service/pkg/pb"
	"time"

	"buf.build/go/protovalidate"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// OrderServer adapts the gRPC API to OrderService.
type OrderServer struct {
	orderpb.UnimplementedOrderServiceServer
	OrderService *service.OrderService
	ZapLogger    *zap.Logger
}

func ts(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func orderToProto(o *model.Order, history []*model.OrderStatusHistory) *orderpb.Order {
	out := &orderpb.Order{
		Id: o.ID, CheckoutId: o.CheckoutID, BuyerId: o.BuyerID, StoreId: o.StoreID, Status: o.Status, PaymentMethod: o.PaymentMethod,
		PaymentStatus: o.PaymentStatus, Subtotal: o.Subtotal, ShippingFee: o.ShippingFee, TotalPrice: o.TotalPrice, Note: o.Note,
		ExpiresAt: ts(o.ExpiresAt), CreatedAt: o.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: o.UpdatedAt.UTC().Format(time.RFC3339),
		PaidAt: ts(o.PaidAt), ShippedAt: ts(o.ShippedAt), DeliveredAt: ts(o.DeliveredAt), CanceledAt: ts(o.CanceledAt),
		CancelReason: o.CancelReason, CanceledBy: o.CanceledBy, Carrier: o.Carrier, TrackingCode: o.TrackingCode, ReturnReason: o.ReturnReason,
	}
	if len(o.ShippingAddress) > 0 {
		var m map[string]any
		if json.Unmarshal(o.ShippingAddress, &m) == nil {
			out.ShippingAddress, _ = structpb.NewStruct(m)
		}
	}
	for _, it := range o.OrderItems {
		out.Items = append(out.Items, &orderpb.OrderItem{
			Id: it.ID, OrderId: it.OrderID, ProductId: it.ProductID, Name: it.ProductName, Sku: it.SKU, ImageUrl: it.ImageURL,
			StoreId: it.StoreID, CategoryId: it.CategoryID, Quantity: it.Quantity, UnitPrice: it.Price, LineTotal: it.LineTotal, Status: it.Status,
		})
	}
	for _, h := range history {
		out.History = append(out.History, &orderpb.StatusHistory{
			FromStatus: h.FromStatus, ToStatus: h.ToStatus, ActorType: h.ActorType, ActorId: h.ActorID, Reason: h.Reason, At: h.At.UTC().Format(time.RFC3339),
		})
	}
	return out
}

func ordersToProto(orders []*model.Order) []*orderpb.Order {
	out := make([]*orderpb.Order, 0, len(orders))
	for _, o := range orders {
		out = append(out, orderToProto(o, nil))
	}
	return out
}

func checkoutToProto(r *service.CheckoutResult) *orderpb.CheckoutResponse {
	return &orderpb.CheckoutResponse{
		CheckoutId: r.Checkout.ID, Orders: ordersToProto(r.Orders), GrandTotal: r.Checkout.Total,
		PaymentMethod: r.Checkout.PaymentMethod, Replayed: r.Replayed,
	}
}

func lines(items []*orderpb.CheckoutItem) []service.Line {
	out := make([]service.Line, 0, len(items))
	for _, it := range items {
		out = append(out, service.Line{ProductID: it.GetProductId(), Quantity: it.GetQuantity()})
	}
	return out
}

func scopeOf(buyerID, storeID uint64, admin bool) repository.Scope {
	return repository.Scope{BuyerID: buyerID, StoreID: storeID, Admin: admin}
}

// Checkout places the orders of a buyer (one per store).
func (s *OrderServer) Checkout(ctx context.Context, req *orderpb.CheckoutRequest) (*orderpb.CheckoutResponse, error) {
	if err := protovalidate.Validate(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	var addr map[string]any
	if req.GetShippingAddress() != nil {
		addr = req.GetShippingAddress().AsMap()
	}
	res, err := s.OrderService.Checkout(ctx, &service.CheckoutInput{
		BuyerID: req.GetBuyerId(), FromCart: req.GetFromCart(), Items: lines(req.GetItems()), PaymentMethod: req.GetPaymentMethod(),
		Address: addr, Note: req.GetNote(), IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, err
	}
	return checkoutToProto(res), nil
}

// PreviewCheckout prices a checkout without creating anything.
func (s *OrderServer) PreviewCheckout(ctx context.Context, req *orderpb.PreviewRequest) (*orderpb.PreviewResponse, error) {
	pv, err := s.OrderService.PreviewCheckout(ctx, req.GetBuyerId(), req.GetFromCart(), lines(req.GetItems()))
	if err != nil {
		return nil, err
	}
	out := &orderpb.PreviewResponse{Subtotal: pv.Subtotal, ShippingFee: pv.ShippingFee, GrandTotal: pv.GrandTotal, CanOrder: pv.CanOrder}
	for _, g := range pv.Groups {
		pg := &orderpb.PreviewGroup{StoreId: g.StoreID, Subtotal: g.Subtotal, ShippingFee: g.ShippingFee, Total: g.Total}
		for _, l := range g.Lines {
			pg.Lines = append(pg.Lines, &orderpb.PreviewLine{
				ProductId: l.ProductID, Name: l.Name, ImageUrl: l.ImageURL, Quantity: l.Quantity, UnitPrice: l.UnitPrice,
				LineTotal: l.LineTotal, StockLevel: l.StockLevel, Available: l.Available, Issue: l.Issue,
			})
		}
		out.Groups = append(out.Groups, pg)
	}
	return out, nil
}

// GetCheckout returns a buyer's checkout.
func (s *OrderServer) GetCheckout(ctx context.Context, req *orderpb.GetCheckoutRequest) (*orderpb.CheckoutResponse, error) {
	res, err := s.OrderService.GetCheckout(ctx, req.GetBuyerId(), req.GetCheckoutId())
	if err != nil {
		return nil, err
	}
	return checkoutToProto(res), nil
}

// GetOrder returns one order in the caller's scope.
func (s *OrderServer) GetOrder(ctx context.Context, req *orderpb.GetOrderRequest) (*orderpb.Order, error) {
	v, err := s.OrderService.GetOrder(ctx, req.GetId(), scopeOf(req.GetBuyerId(), req.GetStoreId(), req.GetIsAdmin()))
	if err != nil {
		return nil, err
	}
	return orderToProto(v.Order, v.History), nil
}

// ListOrders returns a page of orders in scope.
func (s *OrderServer) ListOrders(ctx context.Context, req *orderpb.ListOrdersRequest) (*orderpb.ListOrdersResponse, error) {
	orders, total, err := s.OrderService.ListOrders(ctx, scopeOf(req.GetBuyerId(), req.GetStoreId(), req.GetIsAdmin()), req.GetStatus(), req.GetPage(), req.GetPageSize())
	if err != nil {
		return nil, err
	}
	return &orderpb.ListOrdersResponse{Orders: ordersToProto(orders), Total: uint64(total)}, nil
}

// CountOrders returns the number of orders per status.
func (s *OrderServer) CountOrders(ctx context.Context, req *orderpb.CountOrdersRequest) (*orderpb.CountOrdersResponse, error) {
	m, err := s.OrderService.CountOrders(ctx, scopeOf(req.GetBuyerId(), req.GetStoreId(), req.GetIsAdmin()))
	if err != nil {
		return nil, err
	}
	return &orderpb.CountOrdersResponse{ByStatus: m}, nil
}

// ApplyAction performs a user action on an order.
func (s *OrderServer) ApplyAction(ctx context.Context, req *orderpb.ActionRequest) (*orderpb.Order, error) {
	v, err := s.OrderService.ApplyAction(ctx, &service.ActionInput{
		OrderID: req.GetOrderId(), Action: req.GetAction(), Reason: req.GetReason(), Carrier: req.GetCarrier(), TrackingCode: req.GetTrackingCode(),
		Actor: repository.Actor{Type: req.GetActorType(), ID: req.GetActorId(), StoreID: req.GetStoreId()},
	})
	if err != nil {
		return nil, err
	}
	return orderToProto(v.Order, v.History), nil
}

func cartToProto(c *service.Cart) *orderpb.CartResponse {
	out := &orderpb.CartResponse{Subtotal: c.Subtotal, ItemCount: c.ItemCount}
	for _, l := range c.Lines {
		out.Lines = append(out.Lines, &orderpb.CartLine{
			ProductId: l.ProductID, Quantity: l.Quantity, Name: l.Name, ImageUrl: l.ImageURL, StoreId: l.StoreID, UnitPrice: l.UnitPrice,
			LineTotal: l.LineTotal, StockLevel: l.StockLevel, Available: l.Available, Issue: l.Issue,
		})
	}
	return out
}

// GetCart returns the buyer's cart.
func (s *OrderServer) GetCart(ctx context.Context, req *orderpb.GetCartRequest) (*orderpb.CartResponse, error) {
	c, err := s.OrderService.GetCart(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}
	return cartToProto(c), nil
}

// SetCartItem sets the quantity of one product in the cart.
func (s *OrderServer) SetCartItem(ctx context.Context, req *orderpb.SetCartItemRequest) (*orderpb.CartResponse, error) {
	if err := protovalidate.Validate(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	c, err := s.OrderService.SetCartItem(ctx, req.GetUserId(), req.GetProductId(), req.GetQuantity())
	if err != nil {
		return nil, err
	}
	return cartToProto(c), nil
}

// ClearCart empties the cart.
func (s *OrderServer) ClearCart(ctx context.Context, req *orderpb.ClearCartRequest) (*orderpb.CartResponse, error) {
	c, err := s.OrderService.ClearCart(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}
	return cartToProto(c), nil
}

// MergeCart merges a guest cart after login.
func (s *OrderServer) MergeCart(ctx context.Context, req *orderpb.MergeCartRequest) (*orderpb.CartResponse, error) {
	c, err := s.OrderService.MergeCart(ctx, req.GetUserId(), lines(req.GetItems()))
	if err != nil {
		return nil, err
	}
	return cartToProto(c), nil
}
