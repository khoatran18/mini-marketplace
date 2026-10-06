package repository

import (
	"context"
	"errors"
	"product-service/pkg/model"
	"product-service/pkg/outbox"
	"slices"

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

// UpdateProduct replaces the editable fields of a product. Owner (seller_id) is never changed,
// and zero values (price/inventory 0) are written explicitly.
func (r *ProductRepository) UpdateProduct(ctx context.Context, product *model.Product) error {
	result := r.DB.WithContext(ctx).Model(&model.Product{}).Where("id = ?", product.ID).Updates(map[string]interface{}{
		"name":       product.Name,
		"price":      product.Price,
		"inventory":  product.Inventory,
		"attributes": product.Attributes,
	})
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

// ItemQuantity is a product and the quantity to reserve or release.
type ItemQuantity struct {
	ProductID uint64
	Quantity  int64
}

// ErrInsufficientInventory is returned when a product is missing or has too little stock.
var ErrInsufficientInventory = errors.New("insufficient inventory")

func sortItems(items []ItemQuantity) []ItemQuantity {
	sorted := slices.Clone(items)
	// Fixed lock order across concurrent orders avoids deadlocks
	slices.SortFunc(sorted, func(a, b ItemQuantity) int {
		switch {
		case a.ProductID < b.ProductID:
			return -1
		case a.ProductID > b.ProductID:
			return 1
		}
		return 0
	})
	return sorted
}

// ReserveInventory decrements stock for every item of an order and records the successful
// validation result, all in one transaction. If any item lacks stock nothing is changed and
// ErrInsufficientInventory is returned. A concurrent duplicate delivery of the same order fails on
// the primary key of the result row and rolls back its own decrements.
func (r *ProductRepository) ReserveInventory(ctx context.Context, orderID uint64, items []ItemQuantity) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range sortItems(items) {
			result := tx.Model(&model.Product{}).
				Where("id = ? AND inventory >= ?", item.ProductID, item.Quantity).
				UpdateColumn("inventory", gorm.Expr("inventory - ?", item.Quantity))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrInsufficientInventory
			}
		}
		return tx.Create(&outbox.ValidateOrderEvent{
			OrderID:   orderID,
			Success:   true,
			Status:    "PENDING",
			Processed: true,
		}).Error
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

// MaxPageSize caps the number of products returned per page.
const MaxPageSize = 100

func (r *ProductRepository) GetProducts(ctx context.Context, page, pageSize uint64) ([]*model.Product, error) {
	var products []*model.Product
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}
	pageSizeInt := int(pageSize)
	offset := int((page - 1) * pageSize)
	if err := r.DB.WithContext(ctx).Order("id ASC").Limit(pageSizeInt).Offset(offset).Find(&products).Error; err != nil {
		return nil, err
	}
	return products, nil
}
