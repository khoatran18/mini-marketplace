package repository

import (
	"context"
	"order-service/pkg/outbox"

	"gorm.io/gorm"
)

func (r *OrderRepository) CreateOrderOutbox(tx *gorm.DB, createOrderOutbox *outbox.CreateOrderEvent) error {
	if err := tx.Create(createOrderOutbox).Error; err != nil {
		return err
	}
	return nil
}

// GetCreateOrderEventNotPublish returns up to limit events that still have to be published, oldest first.
func (r *OrderRepository) GetCreateOrderEventNotPublish(ctx context.Context, limit int) ([]*outbox.CreateOrderEvent, error) {
	var events []*outbox.CreateOrderEvent
	err := r.DB.WithContext(ctx).Where("status IN ?", []string{"PENDING", "FAILED"}).
		Order("order_id ASC").Limit(limit).Find(&events).Error
	return events, err
}

func (r *OrderRepository) UpdateCreateOrderEventStatus(ctx context.Context, orderID uint64, status string) error {
	return r.DB.WithContext(ctx).Model(&outbox.CreateOrderEvent{}).Where("order_id = ?", orderID).
		Updates(map[string]interface{}{"status": status}).Error
}

// GetCancelOrderEventNotPublish returns up to limit cancel events that still have to be published.
func (r *OrderRepository) GetCancelOrderEventNotPublish(ctx context.Context, limit int) ([]*outbox.CancelOrderEvent, error) {
	var events []*outbox.CancelOrderEvent
	err := r.DB.WithContext(ctx).Where("status IN ?", []string{"PENDING", "FAILED"}).
		Order("order_id ASC").Limit(limit).Find(&events).Error
	return events, err
}

func (r *OrderRepository) UpdateCancelOrderEventStatus(ctx context.Context, orderID uint64, status string) error {
	return r.DB.WithContext(ctx).Model(&outbox.CancelOrderEvent{}).Where("order_id = ?", orderID).
		Updates(map[string]interface{}{"status": status}).Error
}
