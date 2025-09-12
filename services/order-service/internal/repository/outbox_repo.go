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

func (r *OrderRepository) GetCreateOrderEventNotPublish(limit int) ([]*outbox.CreateOrderEvent, error) {
	var CreOrdEvents []*outbox.CreateOrderEvent
	result := r.DB.Model(&outbox.CreateOrderEvent{}).Where("status IN ?", []string{"PENDING", "FAILED"}).Find(&CreOrdEvents).Limit(limit)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return CreOrdEvents, nil
}

func (r *OrderRepository) UpdateCreateOrderEventStatus(ctx context.Context, orderID uint64, status string) error {
	return r.DB.WithContext(ctx).Model(&outbox.CreateOrderEvent{}).Where("order_id = ?", orderID).
		Updates(map[string]interface{}{"status": status}).Error
}
