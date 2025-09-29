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

func (h *ProductHandler) CreateProduct(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.CreateProductInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("ProductHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
		return
	}

	// Get response and parse to json
	res, err := h.Service.CreateProduct(&req)
	if err != nil {
		h.Logger.Warn("ProductHandler: CreateProduct warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *ProductHandler) UpdateProduct(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.UpdateProductInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("ProductHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
		return
	}

	// Get ID
	idStr := c.Param("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.Logger.Warn("ProductHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
	}
	req.Product.ID = idUint

	// Get response and parse to json
	res, err := h.Service.UpdateProduct(&req)
	if err != nil {
		h.Logger.Warn("ProductHandler: UpdateProduct warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *ProductHandler) GetProductByID(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.GetProductByIDInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("ProductHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
		return
	}

	// Get ID
	idStr := c.Param("id")
	idUint, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		h.Logger.Warn("ProductHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
	}
	req.ProductID = idUint

	// Get response and parse to json
	res, err := h.Service.GetProductByID(&req)
	if err != nil {
		h.Logger.Warn("ProductHandler: GetProductByID warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *ProductHandler) GetProductsBySellerID(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.GetProductsBySellerIDInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("ProductHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
		return
	}

	// Get ID
	sellerIdStr := c.Param("seller_id")
	sellerIdUint, err := strconv.ParseUint(sellerIdStr, 10, 64)
	if err != nil {
		h.Logger.Warn("ProductHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
	}
	req.SellerID = sellerIdUint

	// Get response and parse to json
	res, err := h.Service.GetProductsBySellerID(&req)
	if err != nil {
		h.Logger.Warn("ProductHandler: GetProductsBySellerID warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *ProductHandler) GetProducts(c *gin.Context) {

	// Parse from gin.context json to request dto
	var req dto.GetProductsInput
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("ProductHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
		return
	}

	// Get query
	page, err := getQueryInt(c, "page", 1)
	if err != nil {
		h.Logger.Warn("ProductHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
	}
	pageSize, err := getQueryInt(c, "page_size", 10)
	if err != nil {
		h.Logger.Warn("ProductHandler invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": GetErrorString(err.Error())})
	}
	req.Page = uint64(page)
	req.PageSize = uint64(pageSize)

	// Get response and parse to json
	res, err := h.Service.GetProducts(&req)
	if err != nil {
		h.Logger.Warn("ProductHandler: GetProducts warn", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": GetErrorString(err.Error())})
		return
	}
	c.JSON(http.StatusOK, res)
}
