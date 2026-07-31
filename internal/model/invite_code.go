package model

import "time"

const TableNameInviteCode = "invite_codes"

// InviteCode mapped from table <invite_codes>
type InviteCode struct {
	ID        int64      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Code      string     `gorm:"column:code;unique;not null" json:"code"`
	CreatedBy int64      `gorm:"column:created_by;not null" json:"created_by"`
	UsedBy    *int64     `gorm:"column:used_by" json:"used_by"`
	ExpiresAt time.Time  `gorm:"column:expires_at;not null" json:"expires_at"`
	CreatedAt time.Time  `gorm:"column:created_at;default:now()" json:"created_at"`
}

// TableName InviteCode's table name
func (*InviteCode) TableName() string {
	return TableNameInviteCode
}
