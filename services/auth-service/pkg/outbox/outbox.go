package outbox

import "time"

type PwdVersionEvent struct {
	UserID     uint64    `gorm:"primaryKey"`
	PwdVersion int64     `gorm:"notnull"`
	Status     string    `gorm:"notnull;default:'PENDING';index:idx_status_created_at,priority:1"` // PENDING, FAILED, SUCCESS
	CreatedAt  time.Time `gorm:"notnull;index:idx_status_created_at,priority:2"`
}

type PwdVersionKafkaEvent struct {
	UserID     uint64 `json:"user_id"`
	PwdVersion int64  `json:"pwd_version"`
}
