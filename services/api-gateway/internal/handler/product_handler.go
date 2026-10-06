package handler

import (
	"api-gateway/internal/client/authclient"
	"api-gateway/internal/client/productclient"
	"api-gateway/pkg/dto"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ProductHandler : handler for ProductClient
type ProductHandler struct {
	Auth    *authclient.AuthClient // resolves the caller's store (seller) ID
	Service *productclient.ProductClient
	Logger  *zap.Logger
}

// NewProductHandler create new ProductHandler
func NewProductHandler(service *productclient.ProductClient, logger *zap.Logger) *ProductHandler {
	return &ProductHandler{
		Service: service,
		Logger:  logger,
	}
}

const maxPageSize = 100

// callerStoreID resolves the store of the authenticated user; callers without a store are rejected.
func (h *ProductHandler) callerStoreID(c *gin.Context) (uint64, bool) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "User ID not found in context"})
		return 0, false
	}
	storeID, _, err := h.Auth.GetStoreID(userID)
	if err != nil {
		respondError(c, h.Logger, "ProductHandler: resolve store failed", err)
		return 0, false
	}
	if storeID == 0 {
		c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "Create a store before managing products"})
		return 0, false
	}
	return storeID, true
}

// CreateProduct is responsible for parse create product gin.context request
// CreateProduct godoc
// @Summary CreateProduct
// @Description Create new product
// @Tags product
// @Accept json
// @Produce json
// @Param request body dto.CreateProductInput true "Product DTO to create"
// @Security BearerAuth
// @Success 200 {object} dto.CreateProductOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /products [post]
func (h *ProductHandler) CreateProduct(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.CreateProductInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("ProductHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}

	// The product always belongs to the caller's own store
	storeID, ok := h.callerStoreID(c)
	if !ok {
		return
	}
	req.SellerID = storeID
	if req.Name == "" || req.Price <= 0 || req.Inventory < 0 {
		badRequest(c, "Name is required, price must be positive and inventory must not be negative")
		return
	}

	// Get response and parse to json
	res, err := h.Service.CreateProduct(&req)
	if err != nil {
		respondError(c, h.Logger, "ProductHandler: CreateProduct warn", err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// UpdateProduct is responsible for parse update product gin.context request
// UpdateProduct godoc
// @Summary UpdateProduct
// @Description Update product
// @Tags product
// @Accept json
// @Produce json
// @Param request body dto.UpdateProductInput true "Product DTO to update"
// @Security BearerAuth
// @Param id path integer true "Product ID"
// @Success 200 {object} dto.UpdateProductOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /products/{id} [put]
func (h *ProductHandler) UpdateProduct(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.UpdateProductInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("ProductHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}

	// Get ID
	idStr := c.Param("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.Logger.Warn("ProductHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	if req.Product == nil {
		badRequest(c, "Product is required")
		return
	}
	req.Product.ID = idUint
	if req.Product.Price < 0 || req.Product.Inventory < 0 {
		badRequest(c, "Price and inventory must not be negative")
		return
	}

	// Ownership is checked by the product service against the caller's store,
	// never against a user_id sent by the client.
	storeID, ok := h.callerStoreID(c)
	if !ok {
		return
	}
	req.UserId = storeID
	req.Product.SellerID = storeID

	// Get response and parse to json
	res, err := h.Service.UpdateProduct(&req)
	if err != nil {
		respondError(c, h.Logger, "ProductHandler: UpdateProduct warn", err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// GetProductByID is responsible for parse get product by ID gin.context request
// GetProductByID godoc
// @Summary GetProductByID
// @Description Get product by ID
// @Tags product
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path integer true "Product ID"
// @Success 200 {object} dto.GetProductByIDOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /products/{id} [get]
func (h *ProductHandler) GetProductByID(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.GetProductByIDInput
	//if err := c.ShouldBindJSON(&req); err != nil {
	//	h.Logger.Warn("ProductHandler invalid request", zap.Error(err))
	//	c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
	//	return
	//}

	// Get ID
	idStr := c.Param("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.Logger.Warn("ProductHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	req.ProductID = idUint

	// Get response and parse to json
	res, err := h.Service.GetProductByID(&req)
	if err != nil {
		respondError(c, h.Logger, "ProductHandler: GetProductByID warn", err)
		return
	}
	v, err := resolveViewer(c, h.Auth)
	if err != nil {
		respondError(c, h.Logger, "ProductHandler: resolve viewer warn", err)
		return
	}
	// Drafts, hidden and banned products exist only for their owner (and admins)
	if res.Product != nil && res.Product.Status != "active" && !v.Owns(res.Product.SellerID) {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Product not found"})
		return
	}
	maskProduct(res.Product, v)
	c.JSON(http.StatusOK, res)
}

// GetProductsBySellerID is responsible for parse get product by seller_id gin.context request
// GetProductsBySellerID godoc
// @Summary GetProductsBySellerID
// @Description Get product by seller_id
// @Tags product
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param seller_id path integer true "Seller ID"
// @Success 200 {object} dto.GetProductsBySellerIDOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /products/seller/{seller_id} [get]
func (h *ProductHandler) GetProductsBySellerID(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.GetProductsBySellerIDInput
	//if err := c.ShouldBindJSON(&req); err != nil {
	//	h.Logger.Warn("ProductHandler invalid request", zap.Error(err))
	//	c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
	//	return
	//}

	// Get ID
	sellerIdStr := c.Param("seller_id")
	sellerIdUint, err := strconv.ParseUint(sellerIdStr, 10, 64)
	if err != nil {
		h.Logger.Warn("ProductHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	req.SellerID = sellerIdUint
	v, err := resolveViewer(c, h.Auth)
	if err != nil {
		respondError(c, h.Logger, "ProductHandler: resolve viewer warn", err)
		return
	}
	// Only the store itself (or an admin) sees drafts/hidden products
	req.OnlyActive = !v.Owns(sellerIdUint)

	// Get response and parse to json
	res, err := h.Service.GetProductsBySellerID(&req)
	if err != nil {
		respondError(c, h.Logger, "ProductHandler: GetProductsBySellerID warn", err)
		return
	}
	for _, p := range res.Products {
		maskProduct(p, v)
	}
	c.JSON(http.StatusOK, res)
}

// GetProducts is responsible for parse get products random at home gin.context request
// GetProducts godoc
// @Summary GetProducts
// @Description Get products random at home
// @Tags product
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query integer true "Page number"
// @Param page_size query integer true "Page size"
// @Success 200 {object} dto.GetProductsOutput
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /products [get]
func (h *ProductHandler) GetProducts(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.GetProductsInput
	//if err := c.ShouldBindJSON(&req); err != nil {
	//	h.Logger.Warn("ProductHandler invalid request", zap.Error(err))
	//	c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
	//	return
	//}

	// Get query
	page, err := getQueryInt(c, "page", 1)
	if err != nil {
		h.Logger.Warn("ProductHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	pageSize, err := getQueryInt(c, "page_size", 10)
	if err != nil {
		h.Logger.Warn("ProductHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	req.Page = uint64(page)
	req.PageSize = uint64(pageSize)
	req.OnlyActive = true // the public listing never shows drafts, hidden or banned products

	// Get response and parse to json
	res, err := h.Service.GetProducts(&req)
	if err != nil {
		respondError(c, h.Logger, "ProductHandler: GetProducts warn", err)
		return
	}
	for _, p := range res.Products {
		maskProduct(p, Viewer{})
	}
	c.JSON(http.StatusOK, res)
}
