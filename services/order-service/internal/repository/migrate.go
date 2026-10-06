package repository

import (
	"order-service/pkg/events"
	"order-service/pkg/model"
	"order-service/pkg/outbox"

	"gorm.io/gorm"
)

// Migrate creates/updates every table and migrates legacy data. Idempotent; used by main and by tests.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&model.Order{}, &model.OrderItem{}, &model.OrderStatusHistory{}, &model.Checkout{}, &model.CartItem{},
		&outbox.CreateOrderEvent{}, &outbox.CancelOrderEvent{},
	); err != nil {
		return err
	}
	if err := events.Migrate(db); err != nil {
		return err
	}
	// "SUCCESS" (stock reserved, no payment concept yet) is now CONFIRMED (cash on delivery)
	return db.Model(&model.Order{}).Where("status = ?", model.LegacyStatusSuccess).Update("status", model.StatusConfirmed).Error
}
