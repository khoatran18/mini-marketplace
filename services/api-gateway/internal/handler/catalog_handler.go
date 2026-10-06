package handler

import (
	"api-gateway/internal/client"
	"api-gateway/internal/client/authclient"
	"api-gateway/pkg/dto"
	productpb "api-gateway/pkg/pb/productservice"
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"
)

// CatalogHandler serves the catalog features added on top of the basic product CRUD: search, categories,
// product visibility, manual stock changes and the stock ledger. It calls product-service directly.
type CatalogHandler struct {
	Clients *client.ClientManager
	Auth    *authclient.AuthClient
	Logger  *zap.Logger
}

// NewCatalogHandler creates a CatalogHandler.
func NewCatalogHandler(cm *client.ClientManager, auth *authclient.AuthClient, logger *zap.Logger) *CatalogHandler {
	return &CatalogHandler{Clients: cm, Auth: auth, Logger: logger}
}

// writeProto answers with a protobuf message as JSON: proto field names (snake_case), zero values included and
// 64-bit integers as JSON numbers (protojson would quote them as strings, which is awkward for JavaScript clients).
func writeProto(c *gin.Context, code int, m proto.Message) {
	c.JSON(code, messageToMap(m.ProtoReflect()))
}

func messageToMap(m protoreflect.Message) map[string]any {
	out := map[string]any{}
	fields := m.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		out[string(fd.Name())] = fieldToValue(fd, m.Get(fd))
	}
	return out
}

func fieldToValue(fd protoreflect.FieldDescriptor, v protoreflect.Value) any {
	switch {
	case fd.IsList():
		list := v.List()
		out := make([]any, 0, list.Len())
		for i := 0; i < list.Len(); i++ {
			out = append(out, scalarToValue(fd, list.Get(i)))
		}
		return out
	case fd.IsMap():
		out := map[string]any{}
		v.Map().Range(func(k protoreflect.MapKey, mv protoreflect.Value) bool {
			out[k.String()] = scalarToValue(fd.MapValue(), mv)
			return true
		})
		return out
	}
	return scalarToValue(fd, v)
}

func scalarToValue(fd protoreflect.FieldDescriptor, v protoreflect.Value) any {
	switch fd.Kind() {
	case protoreflect.MessageKind, protoreflect.GroupKind:
		if !v.Message().IsValid() {
			return nil
		}
		if s, ok := v.Message().Interface().(*structpb.Struct); ok {
			return s.AsMap()
		}
		return messageToMap(v.Message())
	case protoreflect.EnumKind:
		return int32(v.Enum())
	case protoreflect.BytesKind:
		return v.Bytes()
	default:
		return v.Interface()
	}
}

func (h *CatalogHandler) product(c *gin.Context) (productpb.ProductServiceClient, context.Context, context.CancelFunc, bool) {
	pc, err := h.Clients.Product()
	if err != nil {
		respondError(c, h.Logger, "CatalogHandler: product client", err)
		return nil, nil, nil, false
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	return pc, ctx, cancel, true
}

func maskProductPB(p *productpb.Product, v Viewer) {
	if p == nil || v.Owns(p.GetSellerId()) {
		return
	}
	if p.Inventory > publicStockCap {
		p.Inventory = publicStockCap
	}
	p.Reserved, p.LowStockThreshold, p.Version = 0, 0, 0
}

func parseUintParam(c *gin.Context, name string) (uint64, bool) {
	v, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || v == 0 {
		badRequest(c, "Invalid "+name)
		return 0, false
	}
	return v, true
}

func queryFloat(c *gin.Context, key string) (float64, bool) {
	raw := c.Query(key)
	if raw == "" {
		return 0, true
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f < 0 {
		badRequest(c, "Invalid "+key)
		return 0, false
	}
	return f, true
}

func queryUint(c *gin.Context, key string, def uint64) (uint64, bool) {
	raw := c.Query(key)
	if raw == "" {
		return def, true
	}
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		badRequest(c, "Invalid "+key)
		return 0, false
	}
	return n, true
}

const maxSearchPageSize = 50

func (h *CatalogHandler) searchRequest(c *gin.Context) (*productpb.SearchProductsRequest, bool) {
	req := &productpb.SearchProductsRequest{Query: strings.TrimSpace(c.Query("q")), Brand: c.Query("brand"), Sort: c.Query("sort")}
	var ok bool
	if req.PriceMin, ok = queryFloat(c, "price_min"); !ok {
		return nil, false
	}
	if req.PriceMax, ok = queryFloat(c, "price_max"); !ok {
		return nil, false
	}
	if req.CategoryId, ok = queryUint(c, "category_id", 0); !ok {
		return nil, false
	}
	if req.SellerId, ok = queryUint(c, "seller_id", 0); !ok {
		return nil, false
	}
	if req.Page, ok = queryUint(c, "page", 1); !ok {
		return nil, false
	}
	if req.PageSize, ok = queryUint(c, "page_size", 20); !ok {
		return nil, false
	}
	if req.PageSize < 1 || req.PageSize > maxSearchPageSize {
		req.PageSize = maxSearchPageSize
	}
	req.InStockOnly = c.Query("in_stock") == "true" || c.Query("in_stock") == "1"
	return req, true
}

// Search godoc
// @Summary Search products
// @Description Keyword search (accent-insensitive, prefix matching) with category/price/brand/stock filters. Public: only active products.
// @Tags catalog
// @Param q query string false "Search text"
// @Param category_id query integer false "Category (includes sub-categories)"
// @Param price_min query number false "Minimum price"
// @Param price_max query number false "Maximum price"
// @Param brand query string false "Brand"
// @Param seller_id query integer false "Store"
// @Param in_stock query boolean false "Only products in stock"
// @Param sort query string false "relevance | price_asc | price_desc | newest | best_selling"
// @Param page query integer false "Page"
// @Param page_size query integer false "Page size (max 50)"
// @Success 200 {object} productpb.SearchProductsResponse
// @Router /search [get]
func (h *CatalogHandler) Search(c *gin.Context) {
	req, ok := h.searchRequest(c)
	if !ok {
		return
	}
	v, err := resolveViewer(c, h.Auth)
	if err != nil {
		respondError(c, h.Logger, "CatalogHandler: resolve viewer", err)
		return
	}
	pc, ctx, cancel, ok := h.product(c)
	if !ok {
		return
	}
	defer cancel()
	// Drafts/hidden products are only ever included for the store itself (or an admin) asking for its own listing
	req.IncludeAllStatuses = req.SellerId != 0 && v.Owns(req.SellerId)
	res, err := pc.SearchProducts(ctx, req)
	if err != nil {
		respondError(c, h.Logger, "CatalogHandler: SearchProducts", err)
		return
	}
	for _, p := range res.Products {
		maskProductPB(p, v)
	}
	writeProto(c, http.StatusOK, res)
}

// SellerProducts godoc
// @Summary List the products of the caller's store (all statuses)
// @Tags seller
// @Security BearerAuth
// @Router /seller/products [get]
func (h *CatalogHandler) SellerProducts(c *gin.Context) {
	v, ok := requireSellerStore(c, h.Auth)
	if !ok {
		return
	}
	req, ok := h.searchRequest(c)
	if !ok {
		return
	}
	pc, ctx, cancel, ok := h.product(c)
	if !ok {
		return
	}
	defer cancel()
	req.SellerId, req.IncludeAllStatuses = v.StoreID, true // the store comes from the token, never from the query
	res, err := pc.SearchProducts(ctx, req)
	if err != nil {
		respondError(c, h.Logger, "CatalogHandler: SearchProducts (seller)", err)
		return
	}
	writeProto(c, http.StatusOK, res)
}

// Categories godoc
// @Summary Category tree (flat list with parent_id)
// @Tags catalog
// @Success 200 {object} productpb.ListCategoriesResponse
// @Router /categories [get]
func (h *CatalogHandler) Categories(c *gin.Context) {
	pc, ctx, cancel, ok := h.product(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := pc.ListCategories(ctx, &productpb.ListCategoriesRequest{})
	if err != nil {
		respondError(c, h.Logger, "CatalogHandler: ListCategories", err)
		return
	}
	c.Header("Cache-Control", "public, max-age=30")
	writeProto(c, http.StatusOK, res)
}

// AdminCategories lists every category including inactive ones.
func (h *CatalogHandler) AdminCategories(c *gin.Context) {
	pc, ctx, cancel, ok := h.product(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := pc.ListCategories(ctx, &productpb.ListCategoriesRequest{IncludeInactive: true})
	if err != nil {
		respondError(c, h.Logger, "CatalogHandler: ListCategories (admin)", err)
		return
	}
	writeProto(c, http.StatusOK, res)
}

type categoryBody struct {
	ParentID uint64 `json:"parent_id"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Sort     int32  `json:"sort"`
	Active   *bool  `json:"active"`
}

func (h *CatalogHandler) upsertCategory(c *gin.Context, id uint64) {
	var body categoryBody
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, GetErrorString(err.Error()))
		return
	}
	active := true
	if body.Active != nil {
		active = *body.Active
	}
	pc, ctx, cancel, ok := h.product(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := pc.UpsertCategory(ctx, &productpb.UpsertCategoryRequest{Category: &productpb.Category{
		Id: id, ParentId: body.ParentID, Name: body.Name, Slug: body.Slug, Sort: body.Sort, Active: active,
	}})
	if err != nil {
		respondError(c, h.Logger, "CatalogHandler: UpsertCategory", err)
		return
	}
	code := http.StatusOK
	if id == 0 {
		code = http.StatusCreated
	}
	writeProto(c, code, res)
}

// CreateCategory (admin) creates a category.
func (h *CatalogHandler) CreateCategory(c *gin.Context) { h.upsertCategory(c, 0) }

// UpdateCategory (admin) replaces a category.
func (h *CatalogHandler) UpdateCategory(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	h.upsertCategory(c, id)
}

type statusBody struct {
	Status string `json:"status" binding:"required"`
}

// SetStatus lets the owner store change a product's visibility (draft | active | hidden).
func (h *CatalogHandler) SetStatus(c *gin.Context) {
	v, ok := requireSellerStore(c, h.Auth)
	if !ok {
		return
	}
	h.setStatus(c, v.StoreID, false)
}

// AdminSetStatus lets an administrator change any product's status, including banned.
func (h *CatalogHandler) AdminSetStatus(c *gin.Context) { h.setStatus(c, 0, true) }

func (h *CatalogHandler) setStatus(c *gin.Context, storeID uint64, admin bool) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var body statusBody
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, GetErrorString(err.Error()))
		return
	}
	pc, ctx, cancel, ok := h.product(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := pc.SetProductStatus(ctx, &productpb.SetProductStatusRequest{ProductId: id, StoreId: storeID, IsAdmin: admin, Status: body.Status})
	if err != nil {
		respondError(c, h.Logger, "CatalogHandler: SetProductStatus", err)
		return
	}
	writeProto(c, http.StatusOK, res)
}

type adjustBody struct {
	Delta  int64  `json:"delta"`
	Reason string `json:"reason"`
}

// AdjustInventory records a manual stock correction for a product of the caller's store.
func (h *CatalogHandler) AdjustInventory(c *gin.Context) {
	v, ok := requireSellerStore(c, h.Auth)
	if !ok {
		return
	}
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var body adjustBody
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, GetErrorString(err.Error()))
		return
	}
	pc, ctx, cancel, ok := h.product(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := pc.AdjustInventory(ctx, &productpb.AdjustInventoryRequest{ProductId: id, StoreId: v.StoreID, Delta: body.Delta, Reason: body.Reason})
	if err != nil {
		respondError(c, h.Logger, "CatalogHandler: AdjustInventory", err)
		return
	}
	writeProto(c, http.StatusOK, res)
}

// InventoryLedger returns the stock history of a product of the caller's store.
func (h *CatalogHandler) InventoryLedger(c *gin.Context) {
	v, ok := requireSellerStore(c, h.Auth)
	if !ok {
		return
	}
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	page, ok := queryUint(c, "page", 1)
	if !ok {
		return
	}
	size, ok := queryUint(c, "page_size", 50)
	if !ok {
		return
	}
	pc, ctx, cancel, ok := h.product(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := pc.GetInventoryLedger(ctx, &productpb.GetInventoryLedgerRequest{ProductId: id, StoreId: v.StoreID, Page: page, PageSize: size})
	if err != nil {
		respondError(c, h.Logger, "CatalogHandler: GetInventoryLedger", err)
		return
	}
	writeProto(c, http.StatusOK, res)
}

// LowStock lists the caller's products at or below their low-stock threshold.
func (h *CatalogHandler) LowStock(c *gin.Context) {
	v, ok := requireSellerStore(c, h.Auth)
	if !ok {
		return
	}
	threshold, ok := queryUint(c, "threshold", 0)
	if !ok {
		return
	}
	pc, ctx, cancel, ok := h.product(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := pc.ListLowStock(ctx, &productpb.ListLowStockRequest{StoreId: v.StoreID, DefaultThreshold: int64(threshold)})
	if err != nil {
		respondError(c, h.Logger, "CatalogHandler: ListLowStock", err)
		return
	}
	writeProto(c, http.StatusOK, res)
}

// Stock reports only the stock level of a product (none | low | ok) to everybody.
func (h *CatalogHandler) Stock(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	pc, ctx, cancel, ok := h.product(c)
	if !ok {
		return
	}
	defer cancel()
	res, err := pc.GetProductByID(ctx, &productpb.GetProductByIDRequest{Id: id})
	if err != nil || res.GetProduct().GetStatus() != "active" {
		if err == nil {
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Product not found"})
			return
		}
		respondError(c, h.Logger, "CatalogHandler: Stock", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"product_id": id, "level": res.GetProduct().GetStockLevel()})
}
