package model

import "gorm.io/gorm"

type Account struct {
	ID         uint64         `gorm:"primaryKey;autoIncrement"`
	Username   string         `gorm:"uniqueIndex:uni_accounts_username_role;not null"`
	Password   string         `gorm:"not null"`
	Role       string         `gorm:"uniqueIndex:uni_accounts_username_role;not null"`
	PwdVersion int64          `gorm:"not null"`
	DeletedAt  gorm.DeletedAt `gorm:"index"`
	StoreID    uint64         `gorm:"default:0"`
}
