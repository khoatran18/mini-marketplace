package repository

import (
	"context"
	"errors"
	"product-service/pkg/model"
	"product-service/pkg/outbox"
	"strconv"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RecordFailedValidation stores a "not enough inventory" result for an order (idempotent).
func (r *ProductRepository) RecordFailedValidation(ctx context.Context, orderID uint64) error {
	event := &outbox.ValidateOrderEvent{
		OrderID:   orderID,
		Success:   false,
		Status:    "PENDING",
		Processed: true,
	}
	return r.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(event).Error
}

// GetProcessedValOrdEvent reports whether a validation result already exists for the order.
func (r *ProductRepository) GetProcessedValOrdEvent(ctx context.Context, orderID uint64) (bool, error) {
	var count int64
	err := r.DB.WithContext(ctx).Model(&outbox.ValidateOrderEvent{}).
		Where("order_id = ? AND processed = ?", orderID, true).Count(&count).Error
	return count > 0, err
}

// GetValOrdEventNotPublish returns up to limit validation results that still have to be published.
func (r *ProductRepository) GetValOrdEventNotPublish(ctx context.Context, limit int) ([]*outbox.ValidateOrderKafkaEvent, error) {
	var events []*outbox.ValidateOrderKafkaEvent
	err := r.DB.WithContext(ctx).Model(&outbox.ValidateOrderEvent{}).
		Where("status IN ? AND processed = ?", []string{"PENDING", "FAILED"}, true).
		Order("order_id ASC").Limit(limit).Find(&events).Error
	return events, err
}

func (r *ProductRepository) UpdateValOrdEventStatus(ctx context.Context, orderID uint64, status string) error {
	return r.DB.WithContext(ctx).Model(&outbox.ValidateOrderEvent{}).Where("order_id = ?", orderID).
		Updates(map[string]interface{}{"status": status}).Error
}

// ReleaseInventory gives the inventory of a canceled/expired/failed order back, exactly once.
// It does nothing when the order never reserved inventory or was already released (or already shipped).
func (r *ProductRepository) ReleaseInventory(ctx context.Context, orderID uint64, items []ItemQuantity) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var event outbox.ValidateOrderEvent
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_id = ?", orderID).First(&event).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if !event.Success || event.Restored || event.Shipped {
			return nil
		}

		ref := strconv.FormatUint(orderID, 10)
		for _, item := range sortItems(items) {
			if _, err := applyStock(tx, item.ProductID, item.Quantity, -item.Quantity, 0, model.ReasonRelease, "order", ref); err != nil {
				return err
			}
		}
		return tx.Model(&outbox.ValidateOrderEvent{}).Where("order_id = ?", orderID).Update("restored", true).Error
	})
}

// MarkShipped records that the reserved goods of an order left the warehouse: reserved goes down and the
// sold counter up (available stock does not change). Exactly once per order; no-op when the order never
// reserved, was released, or was already shipped.
func (r *ProductRepository) MarkShipped(ctx context.Context, orderID uint64, items []ItemQuantity) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var event outbox.ValidateOrderEvent
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_id = ?", orderID).First(&event).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if !event.Success || event.Restored || event.Shipped {
			return nil
		}
		ref := strconv.FormatUint(orderID, 10)
		for _, item := range sortItems(items) {
			if _, err := applyStock(tx, item.ProductID, 0, -item.Quantity, item.Quantity, model.ReasonSale, "order", ref); err != nil {
				return err
			}
		}
		return tx.Model(&outbox.ValidateOrderEvent{}).Where("order_id = ?", orderID).Update("shipped", true).Error
	})
}
