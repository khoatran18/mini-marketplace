package repository

import (
	"context"
	"errors"
	"fmt"
	"product-service/pkg/dto"
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
func (r *ProductRepository) CreateProduct(ctx context.Context, product *model.Product) error {
	return r.DB.WithContext(ctx).Create(product).Error
}

// UpdateProduct update product
func (r *ProductRepository) UpdateProduct(ctx context.Context, product *model.Product) error {
	result := r.DB.WithContext(ctx).Model(&model.Product{}).Where("id = ?", product.ID).Updates(product)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// GetProductByID get product by ProductID
func (r *ProductRepository) GetProductByID(ctx context.Context, productID uint64) (*model.Product, error) {
	var product model.Product
	if err := r.DB.WithContext(ctx).Model(&model.Product{}).Where("id = ?", productID).First(&product).Error; err != nil {
		return nil, err
	}
	return &product, nil
}

// GetProductsByID get product by ProductID
func (r *ProductRepository) GetProductsByID(ctx context.Context, productIDs []uint64) ([]*model.Product, error) {
	var products []*model.Product
	if len(productIDs) == 0 {
		return []*model.Product{}, nil
	}
	if err := r.DB.WithContext(ctx).Model(&model.Product{}).Where("id IN ?", productIDs).Find(&products).Error; err != nil {
		return nil, err
	}
	return products, nil
}

// GetInventoryByID get inventory by ProductID
func (r *ProductRepository) GetInventoryByID(ctx context.Context, productID uint64) (int64, error) {
	var product model.Product
	if err := r.DB.WithContext(ctx).Model(&model.Product{}).Where("id = ?", productID).First(&product).Error; err != nil {
		return 0, err
	}
	return product.Inventory, nil
}

// GetSellerIDByID get SellerID by ProductID
func (r *ProductRepository) GetSellerIDByID(ctx context.Context, productID uint64) (uint64, error) {
	var product model.Product
	if err := r.DB.WithContext(ctx).Model(&model.Product{}).Where("id = ?", productID).First(&product).Error; err != nil {
		return 0, err
	}
	return product.SellerID, nil
}

// GetAndDecreaseInventoryByID get and decrease inventory by ProductID (atomic)
func (r *ProductRepository) GetAndDecreaseInventoryByID(ctx context.Context, id uint64, quantity int64) error {
	// Use dto.Product to use atomic transaction: get and delete inventory
	result := r.DB.WithContext(ctx).Model(&model.Product{}).Where("id = ? AND inventory >= ?", id, quantity).UpdateColumn("inventory", gorm.Expr("inventory - ?", quantity))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("no rows affected")
	}
	return nil
}

func (r *ProductRepository) GetAndDecreaseInventoryByIDBatch(ctx context.Context, kafkaEvent *dto.CreateOrderKafkaEvent) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		itemEvents := kafkaEvent.Items
		for _, item := range itemEvents {
			result := tx.Model(&model.Product{}).
				Where("id = ? AND inventory >= ?", item.ProductID, item.Quantity).
				UpdateColumn("inventory", gorm.Expr("inventory - ?", item.Quantity))

			if result.Error != nil {
				return fmt.Errorf("errorDB %v in ID: %d", result.Error.Error(), item.ProductID)
			}
			if result.RowsAffected == 0 {
				return fmt.Errorf("no product for product_id = %d", item.ProductID)
			}
		}

		// Update in OutboxDB
		if err := r.CreateOrUpdateValOrdEvent(tx, kafkaEvent.OrderID, true, true); err != nil {
			return err
		}
		return nil
	})
}

// GetProductsBySellerID get product array by SellerID
func (r *ProductRepository) GetProductsBySellerID(ctx context.Context, sellerID uint64) ([]*model.Product, error) {
	var products []*model.Product
	if err := r.DB.WithContext(ctx).Where("seller_id = ?", sellerID).Find(&products).Error; err != nil {
		return nil, err
	}
	return products, nil
}

func (r *ProductRepository) GetProducts(ctx context.Context, page, pageSize uint64) ([]*model.Product, error) {
	var products []*model.Product
	pageSizeInt := int(pageSize)
	offset := int((page - 1) * pageSize)
	if err := r.DB.WithContext(ctx).Limit(pageSizeInt).Offset(offset).Find(&products).Error; err != nil {
		return nil, err
	}
	return products, nil
}
