package repository

import (
	"errors"
	"product-service/pkg/events"
	"product-service/pkg/model"
	"strconv"
	"time"

	"gorm.io/gorm"
)

// InventoryChangedEvent is published on TopicInventoryChanged after every change of available stock.
type InventoryChangedEvent struct {
	V         int       `json:"v"`
	ProductID uint64    `json:"product_id"`
	StoreID   uint64    `json:"store_id"`
	Delta     int64     `json:"delta"`
	Available int64     `json:"available"`
	Reserved  int64     `json:"reserved"`
	Level     string    `json:"level"` // none | low | ok
	Reason    string    `json:"reason"`
	RefType   string    `json:"ref_type"`
	RefID     string    `json:"ref_id"`
	At        time.Time `json:"at"`
}

// ProductChangedEvent is published on TopicProductChanged when a catalog entry is created, edited, hidden or banned.
type ProductChangedEvent struct {
	V          int       `json:"v"`
	ProductID  uint64    `json:"product_id"`
	Op         string    `json:"op"` // upsert | delete
	StoreID    uint64    `json:"store_id"`
	Name       string    `json:"name"`
	SKU        string    `json:"sku"`
	CategoryID uint64    `json:"category_id"`
	Brand      string    `json:"brand"`
	Price      float64   `json:"price"`
	Status     string    `json:"status"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type stockRow struct {
	SellerID          uint64
	Inventory         int64
	Reserved          int64
	LowStockThreshold int64
}

// applyStock atomically changes available stock (dInv), reserved stock (dRes, never below 0) and the sold
// counter (dSold) of one product, appends a ledger row and emits inventory.changed. It fails with
// ErrInsufficientInventory when the product does not exist or the available stock would become negative.
func applyStock(tx *gorm.DB, productID uint64, dInv, dRes, dSold int64, reason, refType, refID string) (*stockRow, error) {
	var row stockRow
	res := tx.Raw(`UPDATE products
		SET inventory = inventory + ?, reserved = GREATEST(reserved + ?, 0), sold = sold + ?, version = version + 1, updated_at = now()
		WHERE id = ? AND deleted_at IS NULL AND inventory + ? >= 0
		RETURNING seller_id, inventory, reserved, low_stock_threshold`, dInv, dRes, dSold, productID, dInv).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrInsufficientInventory
	}
	if err := recordStock(tx, productID, &row, dInv, reason, refType, refID); err != nil {
		return nil, err
	}
	return &row, nil
}

// recordStock appends the ledger row and the inventory.changed event for an already applied change.
func recordStock(tx *gorm.DB, productID uint64, row *stockRow, delta int64, reason, refType, refID string) error {
	if err := tx.Create(&model.InventoryLedger{
		ProductID: productID, Delta: delta, Reason: reason, RefType: refType, RefID: refID, BalanceAfter: row.Inventory,
	}).Error; err != nil {
		return err
	}
	p := model.Product{Inventory: row.Inventory, LowStockThreshold: row.LowStockThreshold}
	return events.Emit(tx, TopicInventoryChanged, strconv.FormatUint(productID, 10), InventoryChangedEvent{
		V: 1, ProductID: productID, StoreID: row.SellerID, Delta: delta, Available: row.Inventory, Reserved: row.Reserved,
		Level: p.StockLevel(model.DefaultLowStockThreshold), Reason: reason, RefType: refType, RefID: refID, At: time.Now().UTC(),
	})
}

func emitProductChanged(tx *gorm.DB, p *model.Product, op string) error {
	return events.Emit(tx, TopicProductChanged, strconv.FormatUint(p.ID, 10), ProductChangedEvent{
		V: 1, ProductID: p.ID, Op: op, StoreID: p.SellerID, Name: p.Name, SKU: p.SKU, CategoryID: p.CategoryID,
		Brand: p.Brand, Price: p.Price, Status: p.Status, UpdatedAt: time.Now().UTC(),
	})
}

// ErrNotOwner is returned when a store tries to change a product it does not own.
var ErrNotOwner = errors.New("store does not own the product")
