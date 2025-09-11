package outbox

import "time"

type CreateSellerEvent struct {
	SellerID  uint64    `gorm:"primaryKey"`
	UserID    uint64    `gorm:"notnull"`
	Status    string    `gorm:"notnull;default:'PENDING';index:idx_status_created_at,priority:1"`
	CreatedAt time.Time `gorm:"notnull;index:idx_status_created_at,priority:2"`
}

type CreateSellerKafkaEvent struct {
	SellerID uint64 `gorm:"seller_id"`
	UserID   uint64 `gorm:"user_id"`
}
