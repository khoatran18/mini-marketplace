package handler

import (
	"api-gateway/internal/client/productclient"
	"api-gateway/pkg/dto"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ProductHandler : handler for ProductClient
type ProductHandler struct {
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

	// Get response and parse to json
	res, err := h.Service.CreateProduct(&req)
	if err != nil {
		h.Logger.Warn("ProductHandler: CreateProduct warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
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
	}
	req.Product.ID = idUint

	// Get response and parse to json
	res, err := h.Service.UpdateProduct(&req)
	if err != nil {
		h.Logger.Warn("ProductHandler: UpdateProduct warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
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
	}
	req.ProductID = idUint

	// Get response and parse to json
	res, err := h.Service.GetProductByID(&req)
	if err != nil {
		h.Logger.Warn("ProductHandler: GetProductByID warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
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
	}
	req.SellerID = sellerIdUint

	// Get response and parse to json
	res, err := h.Service.GetProductsBySellerID(&req)
	if err != nil {
		h.Logger.Warn("ProductHandler: GetProductsBySellerID warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
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
	}
	pageSize, err := getQueryInt(c, "page_size", 10)
	if err != nil {
		h.Logger.Warn("ProductHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: GetErrorString(err.Error())})
	}
	req.Page = uint64(page)
	req.PageSize = uint64(pageSize)

	// Get response and parse to json
	res, err := h.Service.GetProducts(&req)
	if err != nil {
		h.Logger.Warn("ProductHandler: GetProducts warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}
