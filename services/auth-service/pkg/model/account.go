package model

import "gorm.io/gorm"

type Account struct {
	ID         uint64         `gorm:"primaryKey;autoIncrement"`
	Username   string         `gorm:"unique;not null"`
	Password   string         `gorm:"not null"`
	Role       string         `gorm:"not null"`
	PwdVersion int64          `gorm:"not null"`
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}
