package repository

import (
	"context"
	"errors"
	"product-service/pkg/model"
	"product-service/pkg/outbox"
	"slices"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(product).Error; err != nil {
			return err
		}
		// product.changed first so consumers know the product before its first stock movement
		if err := emitProductChanged(tx, product, "upsert"); err != nil {
			return err
		}
		if product.Inventory > 0 {
			row := &stockRow{SellerID: product.SellerID, Inventory: product.Inventory, LowStockThreshold: product.LowStockThreshold}
			return recordStock(tx, product.ID, row, product.Inventory, model.ReasonInitial, "product", strconv.FormatUint(product.ID, 10))
		}
		return nil
	})
}

// UpdateProduct replaces the editable fields of a product. Owner (seller_id) is never changed,
// and zero values (price/inventory 0) are written explicitly. A changed inventory is recorded in the
// ledger (reason "update"). Status "banned" can not be set or cleared here (see SetProductStatus).
func (r *ProductRepository) UpdateProduct(ctx context.Context, product *model.Product) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var old model.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", product.ID).First(&old).Error; err != nil {
			return err
		}
		status := product.Status
		if status == "" || old.Status == model.StatusBanned {
			status = old.Status
		}
		err := tx.Model(&model.Product{}).Where("id = ?", product.ID).Updates(map[string]interface{}{
			"name":                product.Name,
			"price":               product.Price,
			"inventory":           product.Inventory,
			"attributes":          product.Attributes,
			"description":         product.Description,
			"category_id":         product.CategoryID,
			"brand":               product.Brand,
			"tags":                product.Tags,
			"image_urls":          product.ImageURLs,
			"status":              status,
			"sku":                 product.SKU,
			"low_stock_threshold": product.LowStockThreshold,
			"weight_g":            product.WeightG,
			"version":             gorm.Expr("version + 1"),
			"updated_at":          time.Now(),
		}).Error
		if err != nil {
			return err
		}
		if delta := product.Inventory - old.Inventory; delta != 0 {
			row := &stockRow{SellerID: old.SellerID, Inventory: product.Inventory, Reserved: old.Reserved, LowStockThreshold: product.LowStockThreshold}
			if err := recordStock(tx, old.ID, row, delta, model.ReasonUpdate, "product", strconv.FormatUint(old.ID, 10)); err != nil {
				return err
			}
		}
		merged := old
		merged.Name, merged.Price, merged.SKU, merged.CategoryID, merged.Brand, merged.Status = product.Name, product.Price, product.SKU, product.CategoryID, product.Brand, status
		return emitProductChanged(tx, &merged, "upsert")
	})
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
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := applyStock(tx, id, -quantity, 0, 0, model.ReasonAdjust, "legacy", ""); err != nil {
			return errors.New("no rows affected")
		}
		return nil
	})
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
		ref := strconv.FormatUint(orderID, 10)
		for _, item := range sortItems(items) {
			if _, err := applyStock(tx, item.ProductID, -item.Quantity, item.Quantity, 0, model.ReasonReserve, "order", ref); err != nil {
				return err
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
func (r *ProductRepository) GetProductsBySellerID(ctx context.Context, sellerID uint64, onlyActive bool) ([]*model.Product, error) {
	var products []*model.Product
	q := r.DB.WithContext(ctx).Where("seller_id = ?", sellerID)
	if onlyActive {
		q = q.Where("status = ?", model.StatusActive)
	}
	if err := q.Order("id ASC").Find(&products).Error; err != nil {
		return nil, err
	}
	return products, nil
}

// MaxPageSize caps the number of products returned per page.
const MaxPageSize = 100

func (r *ProductRepository) GetProducts(ctx context.Context, page, pageSize uint64, onlyActive bool) ([]*model.Product, error) {
	var products []*model.Product
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}
	pageSizeInt := int(pageSize)
	offset := int((page - 1) * pageSize)
	q := r.DB.WithContext(ctx).Order("id ASC").Limit(pageSizeInt).Offset(offset)
	if onlyActive {
		q = q.Where("status = ?", model.StatusActive)
	}
	if err := q.Find(&products).Error; err != nil {
		return nil, err
	}
	return products, nil
}
