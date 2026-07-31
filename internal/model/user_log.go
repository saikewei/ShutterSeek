package model

import "time"

const TableNameUserLog = "user_logs"

// Event types for user activity logs.
const (
	LogEventLogin    = "login"
	LogEventSession  = "session"
	LogEventLogout   = "logout"
)

// UserLog mapped from table <user_logs>
type UserLog struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID    int64     `gorm:"column:user_id;not null" json:"user_id"`
	Username  string    `gorm:"column:username;not null" json:"username"`
	EventType string    `gorm:"column:event_type;not null" json:"event_type"`
	IP        string    `gorm:"column:ip" json:"ip"`
	CreatedAt time.Time `gorm:"column:created_at;default:now()" json:"created_at"`
}

// TableName UserLog's table name
func (*UserLog) TableName() string {
	return TableNameUserLog
}
