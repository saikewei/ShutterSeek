package model

import "time"

// Creation stores private, work-scoped facts and the confirmed proposal.
type Creation struct {
	ID             string    `gorm:"primaryKey" json:"id"`
	UserID         int64     `json:"-"`
	Title          string    `json:"title"`
	PhotoIDs       string    `json:"-"`
	State          string    `json:"-"`
	CurrentVersion string    `json:"current_version"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
type CreationMessage struct {
	ID         string    `gorm:"primaryKey" json:"id"`
	CreationID string    `json:"creation_id"`
	Role       string    `json:"role"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
}
type CreationRun struct {
	ID             string    `gorm:"primaryKey" json:"id"`
	CreationID     string    `json:"creation_id"`
	UserID         int64     `json:"-"`
	BaseVersion    string    `json:"base_version"`
	Action         string    `json:"action"`
	Status         string    `json:"status"`
	CredentialHash string    `json:"-"`
	ExpiresAt      time.Time `json:"-"`
	Report         string    `json:"report"`
	Input          string    `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
type CreationVersion struct {
	ID           string    `gorm:"primaryKey" json:"id"`
	CreationID   string    `json:"creation_id"`
	RunID        string    `gorm:"uniqueIndex" json:"run_id"`
	HTML         string    `json:"-"`
	State        string    `json:"-"`
	Report       string    `json:"report"`
	UsedPhotoIDs string    `json:"used_photo_ids"`
	Verified     bool      `json:"verified"`
	CreatedAt    time.Time `json:"created_at"`
}
type CreationEvent struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	RunID     string    `json:"run_id"`
	Data      string    `json:"data"`
	CreatedAt time.Time `json:"created_at"`
}
type CreationAsset struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	RunID     string    `json:"run_id"`
	Data      []byte    `json:"-"`
	Metadata  string    `json:"metadata"`
	CreatedAt time.Time `json:"created_at"`
}
type CreationCheckpoint struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	RunID     string    `json:"run_id"`
	Payload   string    `json:"payload"`
	UpdatedAt time.Time `json:"updated_at"`
}
