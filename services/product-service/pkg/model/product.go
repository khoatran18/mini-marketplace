package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Product statuses controlled by the seller (or an admin). "Out of stock" is not a status: it is
// derived from Inventory (see StockLevel).
const (
	StatusDraft  = "draft"
	StatusActive = "active"
	StatusHidden = "hidden"
	StatusBanned = "banned" // set by an admin only
)

// Product is a catalog entry. Inventory is the AVAILABLE quantity (what a buyer can still order);
// Reserved is the quantity held for orders that are unpaid or not shipped yet. Physical stock = Inventory + Reserved.
type Product struct {
	ID         uint64         `gorm:"primaryKey;autoIncrement"`
	Name       string         `gorm:"not null"`
	Price      float64        `gorm:"type:numeric(18,2);not null"`
	SellerID   uint64         `gorm:"not null;index;uniqueIndex:idx_seller_sku,where:sku <> ''"`
	Inventory  int64          `gorm:"not null"`
	Attributes datatypes.JSON `gorm:"not null"`

	SKU               string         `gorm:"not null;default:'';uniqueIndex:idx_seller_sku,where:sku <> ''"`
	Description       string         `gorm:"type:text;not null;default:''"`
	CategoryID        uint64         `gorm:"index"`
	Brand             string         `gorm:"not null;default:''"`
	Tags              datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	ImageURLs         datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	Status            string         `gorm:"not null;default:'active';index"`
	LowStockThreshold int64          `gorm:"not null;default:0"`
	WeightG           int64          `gorm:"not null;default:0"`
	Reserved          int64          `gorm:"not null;default:0"`
	Sold              int64          `gorm:"not null;default:0"`
	Version           int64          `gorm:"not null;default:1"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         gorm.DeletedAt `gorm:"index"`
}

// DefaultLowStockThreshold is used when neither the product nor the caller sets one.
const DefaultLowStockThreshold = 5

// StockLevel reports none (0 available), low (at or below the threshold) or ok.
func (p *Product) StockLevel(defaultThreshold int64) string {
	threshold := p.LowStockThreshold
	if threshold <= 0 {
		threshold = defaultThreshold
	}
	switch {
	case p.Inventory <= 0:
		return "none"
	case p.Inventory <= threshold:
		return "low"
	default:
		return "ok"
	}
}

// Category is a node of the (2-3 level) category tree.
type Category struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	ParentID  uint64 `gorm:"not null;default:0;index"`
	Name      string `gorm:"not null"`
	Slug      string `gorm:"not null;uniqueIndex"`
	Sort      int    `gorm:"not null;default:0"`
	Active    bool   `gorm:"not null;default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Ledger reasons.
const (
	ReasonInitial = "initial" // stock set when the product was created
	ReasonReserve = "reserve"
	ReasonRelease = "release"
	ReasonSale    = "sale" // goods left the warehouse (order shipped)
	ReasonRestock = "restock"
	ReasonAdjust  = "adjust"
	ReasonUpdate  = "update" // inventory edited through UpdateProduct
)

// InventoryLedger is the append-only history of AVAILABLE stock changes (Delta may be 0 for a sale,
// which only moves stock from reserved to shipped).
type InventoryLedger struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement"`
	ProductID    uint64    `gorm:"not null;index:idx_ledger_product_at,priority:1"`
	Delta        int64     `gorm:"not null"`
	Reason       string    `gorm:"not null"`
	RefType      string    `gorm:"not null;default:''"`
	RefID        string    `gorm:"not null;default:''"`
	BalanceAfter int64     `gorm:"not null"`
	At           time.Time `gorm:"not null;autoCreateTime;index:idx_ledger_product_at,priority:2"`
}
