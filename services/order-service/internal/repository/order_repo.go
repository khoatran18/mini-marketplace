package repository

import (
	"context"
	"errors"
	"order-service/pkg/model"
	"order-service/pkg/outbox"
	"slices"

	"gorm.io/gorm"
)

var OrderStatus = []string{"PENDING", "SUCCESS", "FAILED", "CANCELED", "VALID"}

type OrderRepository struct {
	DB *gorm.DB
}

func NewOrderRepository(db *gorm.DB) *OrderRepository {
	return &OrderRepository{
		DB: db,
	}
}

// For Create function

func (r *OrderRepository) CreateOrder(ctx context.Context, order *model.Order, createOrderOutbox *outbox.CreateOrderEvent) error {

	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {

		// Create order in OrderDB
		if err := tx.Create(order).Error; err != nil {
			return err
		}

		// Create order in outbox
		if err := r.CreateOrderOutbox(tx, createOrderOutbox); err != nil {
			return err
		}

		return nil
	})

}

// For Get function

func (r *OrderRepository) GetOrderByID(ctx context.Context, id uint64) (*model.Order, error) {
	var order model.Order
	if err := r.DB.WithContext(ctx).
		Preload("OrderItems", func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Where("quantity > ?", 0) // CANCEL is for querying canceled orders
		}).
		Where("id = ?", id).First(&order).Error; err != nil {
		return nil, err
	}

	return &order, nil
}

//func (r *OrderRepository) GetOrderByIDOnly(ctx context.Context, id uint64) (*model.Order, error) {
//	var order model.Order
//	if err := r.DB.WithContext(ctx).Where("id = ?", id).First(&order).Error; err != nil {
//		return nil, err
//	}
//	return &order, nil
//}

func (r *OrderRepository) GetOrdersByBuyerIDStatus(ctx context.Context, buyerID uint64, status string) ([]*model.Order, error) {

	// Check valid status
	if !slices.Contains(OrderStatus, status) {
		return nil, errors.New("status is not valid")
	}

	//
	var orders []*model.Order
	if err := r.DB.WithContext(ctx).Preload("OrderItems", func(db *gorm.DB) *gorm.DB {
		return db.WithContext(ctx).Where("quantity > 0")
	}).Where("buyer_id = ? and status = ?", buyerID, status).Find(&orders).Error; err != nil {
		return nil, err
	}

	// Return valid results
	return orders, nil
}
func (r *OrderRepository) GetOrderItemsByOrderID(ctx context.Context, id uint64) ([]*model.OrderItem, error) {
	var orderItems []*model.OrderItem
	if err := r.DB.WithContext(ctx).Where("order_id = ?", id).Find(&orderItems).Error; err != nil {
		return nil, err
	}
	return orderItems, nil
}

// For Update function

func (r *OrderRepository) UpdateOrderByID(ctx context.Context, order *model.Order) error {
	return r.DB.WithContext(ctx).Where("id = ?", order.ID).Updates(order).Error
}
func (r *OrderRepository) UpdateOrderItemsByID(ctx context.Context, orderItems []*model.OrderItem) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range orderItems {
			if err := tx.Where("id = ?", item.ID).Updates(item).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func (r *OrderRepository) UpdateOrderStatusByID(ctx context.Context, id uint64, status string) error {
	result := r.DB.WithContext(ctx).Model(&model.Order{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status": status,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("no rows affected")
	}
	return nil
}

// For Canceled order function

func (r *OrderRepository) CancelOrderByID(ctx context.Context, id uint64) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {

		// Update status for Order
		if err := tx.Model(&model.Order{}).
			Where("id = ?", id).
			Updates(map[string]interface{}{"status": "CANCELED"}).Error; err != nil {
			return err
		}

		// Update for OrderItems in Order
		if err := tx.Model(&model.OrderItem{}).
			Where("order_id = ?", id).
			Updates(map[string]interface{}{"status": "CANCELED"}).Error; err != nil {
			return err
		}

		return nil
	})
}

// For Delete function

//func (r *OrderRepository) DeleteOrderByID(ctx context.Context, id uint64) error {
//	return r.DB.WithContext(ctx).Where("id = ?", id).Delete(&model.Order{}).Error
//}
//func (r *OrderRepository) DeleteOrderItemByID(ctx context.Context, id uint64) error {
//	return r.DB.WithContext(ctx).Where("id = ?", id).Delete(&model.OrderItem{}).Error
//}
