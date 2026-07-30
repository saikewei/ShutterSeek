package model

import "time"

const TableNameAlbumPhoto = "album_photos"

// AlbumPhoto mapped from table <album_photos>
type AlbumPhoto struct {
	AlbumID   int64     `gorm:"column:album_id;not null" json:"album_id"`
	PhotoID   int64     `gorm:"column:photo_id;not null" json:"photo_id"`
	SortOrder int32     `gorm:"column:sort_order;default:0" json:"sort_order"`
	AddedAt   time.Time `gorm:"column:added_at;default:now()" json:"added_at"`
}

// TableName AlbumPhoto's table name
func (*AlbumPhoto) TableName() string {
	return TableNameAlbumPhoto
}
