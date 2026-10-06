package handler

import (
	"api-gateway/internal/client"
	"api-gateway/internal/client/authclient"
	"api-gateway/pkg/dto"
	orderpb "api-gateway/pkg/pb/orderservice"
	userpb "api-gateway/pkg/pb/userservice"
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/structpb"
)

// OrderHandler serves carts, checkout and the order lifecycle for buyers, sellers and admins. Identity (buyer,
// store, role) always comes from the token; prices, totals, statuses and ownership are decided by order-service.
type OrderHandler struct {
	Clients  *client.ClientManager
	Auth     *authclient.AuthClient
	Logger   *zap.Logger
	Payments PaymentComposer // optional: attaches the payment of a checkout to responses
}

// PaymentComposer adds payment information to checkout responses (implemented by the payment handler).
type PaymentComposer interface {
	// ForCheckout returns the payment of a checkout as a JSON-ready map, or nil when none exists (yet).
	ForCheckout(ctx context.Context, checkoutID string) map[string]any
}

// NewOrderHandler creates an OrderHandler.
func NewOrderHandler(cm *client.ClientManager, auth *authclient.AuthClient, logger *zap.Logger) *OrderHandler {
	return &OrderHandler{Clients: cm, Auth: auth, Logger: logger}
}

func (h *OrderHandler) order(c *gin.Context) (orderpb.OrderServiceClient, context.Context, context.CancelFunc, bool) {
	oc, err := h.Clients.Order()
	if err != nil {
		respondError(c, h.Logger, "OrderHandler: order client", err)
		return nil, nil, nil, false
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	return oc, ctx, cancel, true
}

// buyerID returns the authenticated buyer; any other role gets 403.
func (h *OrderHandler) buyerID(c *gin.Context) (uint64, bool) {
	v, err := resolveViewer(c, nil)
	if err != nil || !v.Authenticated() {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "User ID not found in context"})
		return 0, false
	}
	if v.Role != "buyer" {
		c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "Only buyers can do this"})
		return 0, false
	}
	return v.UserID, true
}

type itemBody struct {
	ProductID uint64 `json:"product_id"`
	Quantity  int64  `json:"quantity"`
}

func toCheckoutItems(items []itemBody) []*orderpb.CheckoutItem {
	out := make([]*orderpb.CheckoutItem, 0, len(items))
	for _, it := range items {
		out = append(out, &orderpb.CheckoutItem{ProductId: it.ProductID, Quantity: it.Quantity})
	}
	return out
}

func pageParams(c *gin.Context) (page, size uint64, ok bool) {
	if page, ok = queryUint(c, "page", 1); !ok {
		return
	}
	if size, ok = queryUint(c, "page_size", 20); !ok {
		return
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size, true
}

// ---- cart --------------------------------------------------------------------------------------

// GetCart returns the buyer's cart.
func (h *OrderHandler) GetCart(c *gin.Context) {
	uid, ok := h.buyerID(c)
	if !ok {
		return
	}
	oc, ctx, cancel, ok := h.order(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := oc.GetCart(ctx, &orderpb.GetCartRequest{UserId: uid})
	if err != nil {
		respondError(c, h.Logger, "OrderHandler: GetCart", err)
		return
	}
	writeProto(c, http.StatusOK, res)
}

type quantityBody struct {
	Quantity int64 `json:"quantity"`
}

// SetCartItem sets the quantity of a product in the cart (0 removes it).
func (h *OrderHandler) SetCartItem(c *gin.Context) {
	uid, ok := h.buyerID(c)
	if !ok {
		return
	}
	pid, ok := parseUintParam(c, "product_id")
	if !ok {
		return
	}
	var body quantityBody
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, GetErrorString(err.Error()))
		return
	}
	oc, ctx, cancel, ok := h.order(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := oc.SetCartItem(ctx, &orderpb.SetCartItemRequest{UserId: uid, ProductId: pid, Quantity: body.Quantity})
	if err != nil {
		respondError(c, h.Logger, "OrderHandler: SetCartItem", err)
		return
	}
	writeProto(c, http.StatusOK, res)
}

// RemoveCartItem removes one product from the cart.
func (h *OrderHandler) RemoveCartItem(c *gin.Context) {
	uid, ok := h.buyerID(c)
	if !ok {
		return
	}
	pid, ok := parseUintParam(c, "product_id")
	if !ok {
		return
	}
	oc, ctx, cancel, ok := h.order(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := oc.SetCartItem(ctx, &orderpb.SetCartItemRequest{UserId: uid, ProductId: pid, Quantity: 0})
	if err != nil {
		respondError(c, h.Logger, "OrderHandler: RemoveCartItem", err)
		return
	}
	writeProto(c, http.StatusOK, res)
}

// ClearCart empties the cart.
func (h *OrderHandler) ClearCart(c *gin.Context) {
	uid, ok := h.buyerID(c)
	if !ok {
		return
	}
	oc, ctx, cancel, ok := h.order(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := oc.ClearCart(ctx, &orderpb.ClearCartRequest{UserId: uid})
	if err != nil {
		respondError(c, h.Logger, "OrderHandler: ClearCart", err)
		return
	}
	writeProto(c, http.StatusOK, res)
}

type mergeBody struct {
	Items []itemBody `json:"items"`
}

// MergeCart merges a guest cart (localStorage) into the stored cart after login.
func (h *OrderHandler) MergeCart(c *gin.Context) {
	uid, ok := h.buyerID(c)
	if !ok {
		return
	}
	var body mergeBody
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, GetErrorString(err.Error()))
		return
	}
	oc, ctx, cancel, ok := h.order(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := oc.MergeCart(ctx, &orderpb.MergeCartRequest{UserId: uid, Items: toCheckoutItems(body.Items)})
	if err != nil {
		respondError(c, h.Logger, "OrderHandler: MergeCart", err)
		return
	}
	writeProto(c, http.StatusOK, res)
}

// ---- checkout ----------------------------------------------------------------------------------

type previewBody struct {
	FromCart bool       `json:"from_cart"`
	Items    []itemBody `json:"items"`
}

// PreviewCheckout prices a checkout on the server without creating anything.
func (h *OrderHandler) PreviewCheckout(c *gin.Context) {
	uid, ok := h.buyerID(c)
	if !ok {
		return
	}
	var body previewBody
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, GetErrorString(err.Error()))
		return
	}
	oc, ctx, cancel, ok := h.order(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := oc.PreviewCheckout(ctx, &orderpb.PreviewRequest{BuyerId: uid, FromCart: body.FromCart, Items: toCheckoutItems(body.Items)})
	if err != nil {
		respondError(c, h.Logger, "OrderHandler: PreviewCheckout", err)
		return
	}
	writeProto(c, http.StatusOK, res)
}

type checkoutBody struct {
	FromCart      bool       `json:"from_cart"`
	Items         []itemBody `json:"items"`
	AddressID     uint64     `json:"address_id"` // 0 = the default address
	PaymentMethod string     `json:"payment_method"`
	Note          string     `json:"note"`
}

// addressSnapshot loads the buyer's address (never anybody else's) and turns it into the snapshot stored on orders.
func (h *OrderHandler) addressSnapshot(ctx context.Context, userID, addressID uint64) (*structpb.Struct, error) {
	uc, err := h.Clients.User()
	if err != nil {
		return nil, err
	}
	res, err := uc.GetAddress(ctx, &userpb.GetAddressRequest{Id: addressID, UserId: userID})
	if err != nil {
		return nil, err
	}
	a := res.GetAddress()
	return structpb.NewStruct(map[string]any{
		"label": a.GetLabel(), "receiver_name": a.GetReceiverName(), "phone": a.GetPhone(), "line1": a.GetLine1(),
		"ward": a.GetWard(), "district": a.GetDistrict(), "city": a.GetCity(),
	})
}

// Checkout places the orders of the cart (or of explicit items): one order per store, prices from the catalog.
// The Idempotency-Key header is required so a retry never creates a second checkout.
func (h *OrderHandler) Checkout(c *gin.Context) {
	uid, ok := h.buyerID(c)
	if !ok {
		return
	}
	key := c.GetHeader("Idempotency-Key")
	if key == "" {
		badRequest(c, "The Idempotency-Key header is required")
		return
	}
	var body checkoutBody
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, GetErrorString(err.Error()))
		return
	}
	oc, ctx, cancel, ok := h.order(c)
	if !ok {
		return
	}
	defer cancel()
	addr, err := h.addressSnapshot(ctx, uid, body.AddressID)
	if err != nil {
		respondError(c, h.Logger, "OrderHandler: address", err)
		return
	}
	res, err := oc.Checkout(ctx, &orderpb.CheckoutRequest{
		BuyerId: uid, FromCart: body.FromCart, Items: toCheckoutItems(body.Items), PaymentMethod: body.PaymentMethod,
		ShippingAddress: addr, Note: body.Note, IdempotencyKey: key,
	})
	if err != nil {
		respondError(c, h.Logger, "OrderHandler: Checkout", err)
		return
	}
	code := http.StatusCreated
	if res.GetReplayed() {
		code = http.StatusOK
	}
	h.writeCheckout(c, code, res)
}

func (h *OrderHandler) writeCheckout(c *gin.Context, code int, res *orderpb.CheckoutResponse) {
	out := messageToMap(res.ProtoReflect())
	if h.Payments != nil {
		if p := h.Payments.ForCheckout(c.Request.Context(), res.GetCheckoutId()); p != nil {
			out["payment"] = p
		}
	}
	c.JSON(code, out)
}

// GetCheckout returns a buyer's checkout with its orders and (when available) its payment.
func (h *OrderHandler) GetCheckout(c *gin.Context) {
	uid, ok := h.buyerID(c)
	if !ok {
		return
	}
	oc, ctx, cancel, ok := h.order(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := oc.GetCheckout(ctx, &orderpb.GetCheckoutRequest{CheckoutId: c.Param("id"), BuyerId: uid})
	if err != nil {
		respondError(c, h.Logger, "OrderHandler: GetCheckout", err)
		return
	}
	h.writeCheckout(c, http.StatusOK, res)
}

// ---- orders --------------------------------------------------------------------------------------

// scope identifies what the caller may see: "buyer", "seller" or "admin" views.
func (h *OrderHandler) scope(c *gin.Context, kind string) (buyer, store uint64, admin bool, ok bool) {
	v, err := resolveViewer(c, h.Auth)
	if err != nil {
		respondError(c, h.Logger, "OrderHandler: resolve viewer", err)
		return 0, 0, false, false
	}
	switch kind {
	case "buyer":
		if v.Role != "buyer" {
			c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "Only buyers can do this"})
			return 0, 0, false, false
		}
		return v.UserID, 0, false, true
	case "seller":
		if !v.IsSeller() || v.StoreID == 0 {
			c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "Only sellers with a store can do this"})
			return 0, 0, false, false
		}
		return 0, v.StoreID, false, true
	default:
		if !v.IsAdmin() {
			c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "Only administrators can do this"})
			return 0, 0, false, false
		}
		return 0, 0, true, true
	}
}

func (h *OrderHandler) listOrders(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		buyer, store, admin, ok := h.scope(c, kind)
		if !ok {
			return
		}
		page, size, ok := pageParams(c)
		if !ok {
			return
		}
		oc, ctx, cancel, ok := h.order(c)
		if !ok {
			return
		}
		defer cancel()
		res, err := oc.ListOrders(ctx, &orderpb.ListOrdersRequest{BuyerId: buyer, StoreId: store, IsAdmin: admin, Status: c.Query("status"), Page: page, PageSize: size})
		if err != nil {
			respondError(c, h.Logger, "OrderHandler: ListOrders", err)
			return
		}
		writeProto(c, http.StatusOK, res)
	}
}

func (h *OrderHandler) getOrder(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		buyer, store, admin, ok := h.scope(c, kind)
		if !ok {
			return
		}
		id, ok := parseUintParam(c, "id")
		if !ok {
			return
		}
		oc, ctx, cancel, ok := h.order(c)
		if !ok {
			return
		}
		defer cancel()
		res, err := oc.GetOrder(ctx, &orderpb.GetOrderRequest{Id: id, BuyerId: buyer, StoreId: store, IsAdmin: admin})
		if err != nil {
			respondError(c, h.Logger, "OrderHandler: GetOrder", err)
			return
		}
		writeProto(c, http.StatusOK, res)
	}
}

func (h *OrderHandler) countOrders(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		buyer, store, admin, ok := h.scope(c, kind)
		if !ok {
			return
		}
		oc, ctx, cancel, ok := h.order(c)
		if !ok {
			return
		}
		defer cancel()
		res, err := oc.CountOrders(ctx, &orderpb.CountOrdersRequest{BuyerId: buyer, StoreId: store, IsAdmin: admin})
		if err != nil {
			respondError(c, h.Logger, "OrderHandler: CountOrders", err)
			return
		}
		writeProto(c, http.StatusOK, res)
	}
}

// Buyer views.
func (h *OrderHandler) ListMyOrders() gin.HandlerFunc     { return h.listOrders("buyer") }
func (h *OrderHandler) GetMyOrder() gin.HandlerFunc       { return h.getOrder("buyer") }
func (h *OrderHandler) CountMyOrders() gin.HandlerFunc    { return h.countOrders("buyer") }
func (h *OrderHandler) ListStoreOrders() gin.HandlerFunc  { return h.listOrders("seller") }
func (h *OrderHandler) GetStoreOrder() gin.HandlerFunc    { return h.getOrder("seller") }
func (h *OrderHandler) CountStoreOrders() gin.HandlerFunc { return h.countOrders("seller") }
func (h *OrderHandler) ListAllOrders() gin.HandlerFunc    { return h.listOrders("admin") }
func (h *OrderHandler) GetAnyOrder() gin.HandlerFunc      { return h.getOrder("admin") }

type actionBody struct {
	Reason       string `json:"reason"`
	Carrier      string `json:"carrier"`
	TrackingCode string `json:"tracking_code"`
}

// act returns a handler performing action as the given kind of caller. The actor is derived from the token only.
func (h *OrderHandler) act(kind, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		v, err := resolveViewer(c, h.Auth)
		if err != nil {
			respondError(c, h.Logger, "OrderHandler: resolve viewer", err)
			return
		}
		req := &orderpb.ActionRequest{Action: action, ActorId: v.UserID}
		switch kind {
		case "buyer":
			if v.Role != "buyer" {
				c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "Only buyers can do this"})
				return
			}
			req.ActorType = "buyer"
		case "seller":
			if !v.IsSeller() || v.StoreID == 0 {
				c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "Only sellers with a store can do this"})
				return
			}
			req.ActorType, req.StoreId = "seller", v.StoreID
		default:
			if !v.IsAdmin() {
				c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "Only administrators can do this"})
				return
			}
			req.ActorType = "admin"
		}
		id, ok := parseUintParam(c, "id")
		if !ok {
			return
		}
		req.OrderId = id
		var body actionBody
		if c.Request.ContentLength != 0 {
			if err := c.ShouldBindJSON(&body); err != nil {
				badRequest(c, GetErrorString(err.Error()))
				return
			}
		}
		req.Reason, req.Carrier, req.TrackingCode = body.Reason, body.Carrier, body.TrackingCode
		oc, ctx, cancel, ok := h.order(c)
		if !ok {
			return
		}
		defer cancel()
		res, err := oc.ApplyAction(ctx, req)
		if err != nil {
			respondError(c, h.Logger, "OrderHandler: "+kind+" "+action, err)
			return
		}
		writeProto(c, http.StatusOK, res)
	}
}

// Buyer actions.
func (h *OrderHandler) BuyerCancel() gin.HandlerFunc { return h.act("buyer", "cancel") }
func (h *OrderHandler) BuyerConfirmReceived() gin.HandlerFunc {
	return h.act("buyer", "confirm_received")
}
func (h *OrderHandler) BuyerRequestReturn() gin.HandlerFunc { return h.act("buyer", "request_return") }

// Seller actions.
func (h *OrderHandler) SellerShip() gin.HandlerFunc    { return h.act("seller", "ship") }
func (h *OrderHandler) SellerDeliver() gin.HandlerFunc { return h.act("seller", "deliver") }
func (h *OrderHandler) SellerCancel() gin.HandlerFunc  { return h.act("seller", "cancel") }
func (h *OrderHandler) SellerApproveReturn() gin.HandlerFunc {
	return h.act("seller", "approve_return")
}
func (h *OrderHandler) SellerRejectReturn() gin.HandlerFunc { return h.act("seller", "reject_return") }

// Admin actions.
func (h *OrderHandler) AdminCancel() gin.HandlerFunc        { return h.act("admin", "cancel") }
func (h *OrderHandler) AdminDeliver() gin.HandlerFunc       { return h.act("admin", "deliver") }
func (h *OrderHandler) AdminApproveReturn() gin.HandlerFunc { return h.act("admin", "approve_return") }
func (h *OrderHandler) AdminRejectReturn() gin.HandlerFunc  { return h.act("admin", "reject_return") }

// ---- addresses ---------------------------------------------------------------------------------

type addressBody struct {
	Label        string `json:"label"`
	ReceiverName string `json:"receiver_name"`
	Phone        string `json:"phone"`
	Line1        string `json:"line1"`
	Ward         string `json:"ward"`
	District     string `json:"district"`
	City         string `json:"city"`
	IsDefault    bool   `json:"is_default"`
}

func (h *OrderHandler) user(c *gin.Context) (userpb.UserServiceClient, context.Context, context.CancelFunc, bool) {
	uc, err := h.Clients.User()
	if err != nil {
		respondError(c, h.Logger, "OrderHandler: user client", err)
		return nil, nil, nil, false
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	return uc, ctx, cancel, true
}

// ListAddresses returns the caller's delivery addresses.
func (h *OrderHandler) ListAddresses(c *gin.Context) {
	uid, ok := h.buyerID(c)
	if !ok {
		return
	}
	uc, ctx, cancel, ok := h.user(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := uc.ListAddresses(ctx, &userpb.ListAddressesRequest{UserId: uid})
	if err != nil {
		respondError(c, h.Logger, "OrderHandler: ListAddresses", err)
		return
	}
	writeProto(c, http.StatusOK, res)
}

func (h *OrderHandler) upsertAddress(c *gin.Context, id uint64) {
	uid, ok := h.buyerID(c)
	if !ok {
		return
	}
	var body addressBody
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, GetErrorString(err.Error()))
		return
	}
	uc, ctx, cancel, ok := h.user(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := uc.UpsertAddress(ctx, &userpb.UpsertAddressRequest{Address: &userpb.Address{
		Id: id, UserId: uid, Label: body.Label, ReceiverName: body.ReceiverName, Phone: body.Phone, Line1: body.Line1,
		Ward: body.Ward, District: body.District, City: body.City, IsDefault: body.IsDefault,
	}})
	if err != nil {
		respondError(c, h.Logger, "OrderHandler: UpsertAddress", err)
		return
	}
	code := http.StatusOK
	if id == 0 {
		code = http.StatusCreated
	}
	writeProto(c, code, res)
}

// CreateAddress adds a delivery address.
func (h *OrderHandler) CreateAddress(c *gin.Context) { h.upsertAddress(c, 0) }

// UpdateAddress replaces a delivery address of the caller.
func (h *OrderHandler) UpdateAddress(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		badRequest(c, "Invalid id")
		return
	}
	h.upsertAddress(c, id)
}

// DeleteAddress removes a delivery address of the caller.
func (h *OrderHandler) DeleteAddress(c *gin.Context) {
	uid, ok := h.buyerID(c)
	if !ok {
		return
	}
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	uc, ctx, cancel, ok := h.user(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := uc.DeleteAddress(ctx, &userpb.DeleteAddressRequest{Id: id, UserId: uid})
	if err != nil {
		respondError(c, h.Logger, "OrderHandler: DeleteAddress", err)
		return
	}
	writeProto(c, http.StatusOK, res)
}
