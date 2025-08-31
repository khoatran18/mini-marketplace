package repository

import (
	"errors"
	"product-service/pkg/model"

	"gorm.io/gorm"
)

type ProductRepository struct {
	DB *gorm.DB
}

// NewProductRepository create new ProductRepository, mainly used for ProductService
func NewProductRepository(db *gorm.DB) *ProductRepository {
	return &ProductRepository{
		DB: db,
	}
}

// CreateProduct create new product
func (r *ProductRepository) CreateProduct(product *model.Product) error {
	return r.DB.Create(product).Error
}

// UpdateProduct update product
func (r *ProductRepository) UpdateProduct(product *model.Product) error {
	result := r.DB.Model(&model.Product{}).Where("id = ?", product.ID).Updates(product)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// GetProductByID get product by ProductID
func (r *ProductRepository) GetProductByID(productID uint64) (*model.Product, error) {
	var product model.Product
	if err := r.DB.Model(&model.Product{}).Where("id = ?", productID).First(&product).Error; err != nil {
		return nil, err
	}
	return &product, nil
}

// GetInventoryByID get inventory by ProductID
func (r *ProductRepository) GetInventoryByID(productID uint64) (int64, error) {
	var product model.Product
	if err := r.DB.Model(&model.Product{}).Where("id = ?", productID).First(&product).Error; err != nil {
		return 0, err
	}
	return product.Inventory, nil
}

// GetSellerIDByID get SellerID by ProductID
func (r *ProductRepository) GetSellerIDByID(productID uint64) (uint64, error) {
	var product model.Product
	if err := r.DB.Model(&model.Product{}).Where("id = ?", productID).First(&product).Error; err != nil {
		return 0, err
	}
	return product.SellerID, nil
}

// GetAndDecreaseInventoryByID get and decrease inventory by ProductID (atomic)
func (r *ProductRepository) GetAndDecreaseInventoryByID(id uint64, quantity int64) error {
	// Use dto.Product to use atomic transaction: get and delete inventory
	result := r.DB.Model(&model.Product{}).Where("id = ? AND inventory >= ?", id, quantity).UpdateColumn("inventory", gorm.Expr("inventory - ?", quantity))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("no rows affected")
	}
	return nil
}

// GetProductsBySellerID get product array by SellerID
func (r *ProductRepository) GetProductsBySellerID(sellerID uint64) ([]*model.Product, error) {
	var products []*model.Product
	if err := r.DB.Where("seller_id = ?", sellerID).Find(&products).Error; err != nil {
		return nil, err
	}
	return products, nil
}
