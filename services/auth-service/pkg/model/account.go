package model

type Account struct {
	ID         uint   `gorm:"primaryKey;autoIncrement"`
	Username   string `gorm:"unique;not null"`
	Password   string `gorm:"not null"`
	Role       string `gorm:"not null"`
	PwdVersion int    `gorm:"not null"`
}
