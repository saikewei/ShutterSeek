package model

import "time"

const TableNameUser = "users"

// User mapped from table <users>
type User struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Username     string    `gorm:"column:username;unique;not null" json:"username"`
	PasswordHash string    `gorm:"column:password_hash;not null" json:"-"`
	Role         string    `gorm:"column:role;not null;default:guest" json:"role"`
	CreatedAt    time.Time `gorm:"column:created_at;default:now()" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at;default:now()" json:"updated_at"`
}

// TableName User's table name
func (*User) TableName() string {
	return TableNameUser
}
