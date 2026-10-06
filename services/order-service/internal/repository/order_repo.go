package repository

import (
	"context"
	"encoding/json"
	"errors"
	"order-service/pkg/model"
	"order-service/pkg/outbox"
	"slices"

	"gorm.io/gorm"
)

var OrderStatus = []string{model.StatusPending, model.StatusSuccess, model.StatusFailed, model.StatusCanceled}

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

		// Create order in OrderDB (this also assigns order.ID and the item IDs)
		if err := tx.Create(order).Error; err != nil {
			return err
		}

		// The outbox event must reference the real order ID
		createOrderOutbox.OrderID = order.ID

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

// TransitionStatus moves an order to status `to` only if it currently has one of the `from` statuses.
// It returns false when no row matched (order missing or already in another state), which makes
// repeated or out-of-order status events harmless.
func (r *OrderRepository) TransitionStatus(ctx context.Context, id uint64, from []string, to string) (bool, error) {
	result := r.DB.WithContext(ctx).Model(&model.Order{}).
		Where("id = ? AND status IN ?", id, from).
		Updates(map[string]interface{}{"status": to})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// ErrOrderNotFound and ErrInvalidTransition are returned by CancelOrderByID.
var (
	ErrOrderNotFound     = errors.New("order not found")
	ErrInvalidTransition = errors.New("order cannot be canceled in its current status")
)

// CancelOrderByID cancels a confirmed (SUCCESS) order and, in the same transaction, records an
// outbox event so product-service releases the reserved inventory.
func (r *OrderRepository) CancelOrderByID(ctx context.Context, id uint64) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {

		result := tx.Model(&model.Order{}).
			Where("id = ? AND status = ?", id, model.StatusSuccess).
			Updates(map[string]interface{}{"status": model.StatusCanceled})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			var count int64
			if err := tx.Model(&model.Order{}).Where("id = ?", id).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return ErrOrderNotFound
			}
			return ErrInvalidTransition
		}

		// Update for OrderItems in Order
		if err := tx.Model(&model.OrderItem{}).
			Where("order_id = ?", id).
			Updates(map[string]interface{}{"status": "CANCELED"}).Error; err != nil {
			return err
		}

		// Tell product-service to give the inventory back
		var items []*model.OrderItem
		if err := tx.Where("order_id = ?", id).Find(&items).Error; err != nil {
			return err
		}
		eventItems := make([]*outbox.ItemEvent, 0, len(items))
		for _, item := range items {
			eventItems = append(eventItems, &outbox.ItemEvent{ProductID: item.ProductID, Quantity: item.Quantity})
		}
		payload, err := json.Marshal(eventItems)
		if err != nil {
			return err
		}
		return tx.Create(&outbox.CancelOrderEvent{OrderID: id, Items: payload, Status: "PENDING"}).Error
	})
}

// For Delete function

//func (r *OrderRepository) DeleteOrderByID(ctx context.Context, id uint64) error {
//	return r.DB.WithContext(ctx).Where("id = ?", id).Delete(&model.Order{}).Error
//}
//func (r *OrderRepository) DeleteOrderItemByID(ctx context.Context, id uint64) error {
//	return r.DB.WithContext(ctx).Where("id = ?", id).Delete(&model.OrderItem{}).Error
//}
