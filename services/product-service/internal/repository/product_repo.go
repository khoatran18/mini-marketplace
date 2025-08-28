package repository

import (
	"account-service/pkg/model"
	"errors"

	"gorm.io/gorm"
)

type ProductRepository struct {
	DB *gorm.DB
}

func NewProductRepository(db *gorm.DB) *ProductRepository {
	return &ProductRepository{
		DB: db,
	}
}

func (r *ProductRepository) CreateProduct(product *model.Product) error {
	return r.DB.Create(product).Error
}

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

func (r *ProductRepository) GetProductByID(productID uint64) (*model.Product, error) {
	var product model.Product
	if err := r.DB.Model(&model.Product{}).Where("id = ?", productID).First(&product).Error; err != nil {
		return nil, err
	}
	return &product, nil
}

func (r *ProductRepository) GetInventoryByID(productID uint64) (int64, error) {
	var product model.Product
	if err := r.DB.Model(&model.Product{}).Where("id = ?", productID).Find(&product).Error; err != nil {
		return 0, err
	}
	return product.Inventory, nil
}

func (r *ProductRepository) GetSellerIDByID(productID uint64) (uint64, error) {
	var product model.Product
	if err := r.DB.Model(&model.Product{}).Where("id = ?", productID).Find(&product).Error; err != nil {
		return 0, err
	}

	return product.SellerID, nil
}

func (r *ProductRepository) GetAndDecreaseInventoryByID(id uint64, quantity int) error {
	// Use model.Product to use atomic transaction: get and delete inventory
	result := r.DB.Model(&model.Product{}).Where("id = ? AND inventory >= ?", id, quantity).UpdateColumn("inventory", gorm.Expr("inventory - ?", quantity))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("no rows affected")
	}
	return nil
}

func (r *ProductRepository) GetProductsBySellerID(sellerID uint64) ([]model.Product, error) {
	var products []model.Product
	if err := r.DB.Where("seller_id = ?", sellerID).Find(&products).Error; err != nil {
		return nil, err
	}
	return products, nil
}
