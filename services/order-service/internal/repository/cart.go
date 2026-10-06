package repository

import (
	"context"
	"errors"
	"order-service/pkg/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MaxCartQuantity is the largest quantity of one product in a cart.
const MaxCartQuantity = 99

// ErrCartFull is returned when a cart would exceed model.MaxCartLines products.
var ErrCartFull = errors.New("cart is full")

// GetCartItems returns the user's cart lines, oldest first.
func (r *OrderRepository) GetCartItems(ctx context.Context, userID uint64) ([]*model.CartItem, error) {
	var items []*model.CartItem
	err := r.DB.WithContext(ctx).Where("user_id = ?", userID).Order("added_at ASC, product_id ASC").Find(&items).Error
	return items, err
}

// SetCartItem sets the quantity of a product in the cart (0 removes the line).
func (r *OrderRepository) SetCartItem(ctx context.Context, userID, productID uint64, quantity int64) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if quantity <= 0 {
			return tx.Where("user_id = ? AND product_id = ?", userID, productID).Delete(&model.CartItem{}).Error
		}
		return upsertCartLine(tx, userID, productID, quantity, false)
	})
}

// upsertCartLine inserts or updates a line; add=true adds to the existing quantity (capped), otherwise replaces it.
func upsertCartLine(tx *gorm.DB, userID, productID uint64, quantity int64, add bool) error {
	if quantity > MaxCartQuantity {
		quantity = MaxCartQuantity
	}
	var existing model.CartItem
	err := tx.Where("user_id = ? AND product_id = ?", userID, productID).First(&existing).Error
	switch {
	case err == nil:
		next := quantity
		if add {
			next = existing.Quantity + quantity
			if next > MaxCartQuantity {
				next = MaxCartQuantity
			}
		}
		return tx.Model(&existing).Where("user_id = ? AND product_id = ?", userID, productID).Update("quantity", next).Error
	case errors.Is(err, gorm.ErrRecordNotFound):
		var n int64
		if err := tx.Model(&model.CartItem{}).Where("user_id = ?", userID).Count(&n).Error; err != nil {
			return err
		}
		if n >= model.MaxCartLines {
			return ErrCartFull
		}
		return tx.Create(&model.CartItem{UserID: userID, ProductID: productID, Quantity: quantity}).Error
	}
	return err
}

// MergeCart adds guest-cart lines (after login) to the stored cart, capping quantities and the number of lines.
// Lines that do not fit are dropped silently rather than failing the login flow.
func (r *OrderRepository) MergeCart(ctx context.Context, userID uint64, lines map[uint64]int64) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// serialise merges of the same user
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", userID).Find(&[]model.CartItem{}).Error; err != nil {
			return err
		}
		for productID, qty := range lines {
			if qty <= 0 {
				continue
			}
			if err := upsertCartLine(tx, userID, productID, qty, true); err != nil && !errors.Is(err, ErrCartFull) {
				return err
			}
		}
		return nil
	})
}

// ClearCart removes every line of the user's cart.
func (r *OrderRepository) ClearCart(ctx context.Context, userID uint64) error {
	return r.DB.WithContext(ctx).Where("user_id = ?", userID).Delete(&model.CartItem{}).Error
}
