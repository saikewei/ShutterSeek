package model

import "time"

const TableNameAlbum = "albums"

// Album mapped from table <albums>
type Album struct {
	ID           int64     `gorm:"column:id;not null" json:"id"`
	Title        string    `gorm:"column:title;not null" json:"title"`
	Description  string    `gorm:"column:description" json:"description"`
	CoverPhotoID *int64    `gorm:"column:cover_photo_id" json:"cover_photo_id"`
	SortOrder    int32     `gorm:"column:sort_order;default:0" json:"sort_order"`
	IsPublic     bool      `gorm:"column:is_public;not null;default:false" json:"is_public"`
	CreatedAt    time.Time `gorm:"column:created_at;default:now()" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at;default:now()" json:"updated_at"`
}

// TableName Album's table name
func (*Album) TableName() string {
	return TableNameAlbum
}
