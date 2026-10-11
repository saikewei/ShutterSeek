package model

import "time"

// CreationShare grants authenticated, read-only access to one selected version.
type CreationShare struct {
	ID         string    `gorm:"primaryKey" json:"id"`
	CreationID string    `json:"-"`
	VersionID  string    `json:"version_id"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
