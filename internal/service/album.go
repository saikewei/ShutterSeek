package service

import (
	"strconv"
	"time"

	"gorm.io/gorm"

	"shutterseek/internal/model"
)

// AlbumService handles album business logic.
type AlbumService struct {
	DB *gorm.DB
}

// NewAlbumService creates a new AlbumService.
func NewAlbumService(db *gorm.DB) *AlbumService {
	return &AlbumService{DB: db}
}

// AlbumItem is the API-facing representation of an album.
type AlbumItem struct {
	ID           int64     `json:"id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	CoverURL     string    `json:"cover_url"`
	PhotoCount   int64     `json:"photo_count"`
	SortOrder    int32     `json:"sort_order"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AlbumPhotoPage holds a page of photos within an album.
type AlbumPhotoPage struct {
	Photos  []model.Photo
	Total   int64
	HasMore bool
}

// ListAlbums returns all albums with cover URL and photo count.
func (s *AlbumService) ListAlbums() ([]AlbumItem, error) {
	var albums []model.Album
	if err := s.DB.Order("sort_order, id").Find(&albums).Error; err != nil {
		return nil, err
	}

	items := make([]AlbumItem, len(albums))
	for i, a := range albums {
		items[i] = AlbumItem{
			ID:          a.ID,
			Title:       a.Title,
			Description: a.Description,
			SortOrder:   a.SortOrder,
			CreatedAt:   a.CreatedAt,
			UpdatedAt:   a.UpdatedAt,
		}

		var count int64
		s.DB.Model(&model.AlbumPhoto{}).Where("album_id = ?", a.ID).Count(&count)
		items[i].PhotoCount = count

		items[i].CoverURL = s.coverURL(a.ID, a.CoverPhotoID)
	}
	return items, nil
}

// GetAlbum returns a single album detail.
func (s *AlbumService) GetAlbum(id int64) (*AlbumItem, error) {
	var a model.Album
	if err := s.DB.First(&a, id).Error; err != nil {
		return nil, err
	}

	item := &AlbumItem{
		ID:          a.ID,
		Title:       a.Title,
		Description: a.Description,
		SortOrder:   a.SortOrder,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
	}

	var count int64
	s.DB.Model(&model.AlbumPhoto{}).Where("album_id = ?", a.ID).Count(&count)
	item.PhotoCount = count

	item.CoverURL = s.coverURL(a.ID, a.CoverPhotoID)
	return item, nil
}

// ListAlbumPhotos returns a page of photos within an album (cursor-based).
func (s *AlbumService) ListAlbumPhotos(albumID int64, limit int, afterTime time.Time, afterID int64) (*AlbumPhotoPage, error) {
	var total int64
	s.DB.Model(&model.AlbumPhoto{}).Where("album_id = ?", albumID).Count(&total)

	sub := s.DB.Model(&model.AlbumPhoto{}).
		Select("photo_id").
		Where("album_id = ?", albumID)

	q := s.DB.Where("id IN (?)", sub).
		Order("taken_at DESC NULLS LAST, id DESC").
		Limit(limit + 1)

	if !afterTime.IsZero() {
		q = q.Where("(taken_at, id) < (?, ?)", afterTime, afterID)
	} else if afterID > 0 {
		q = q.Where("taken_at IS NULL AND id < ?", afterID)
	}

	var photos []model.Photo
	if err := q.Find(&photos).Error; err != nil {
		return nil, err
	}

	hasMore := len(photos) > limit
	if hasMore {
		photos = photos[:limit]
	}

	return &AlbumPhotoPage{Photos: photos, Total: total, HasMore: hasMore}, nil
}

// coverURL returns the thumbnail URL for the cover photo.
// If coverID is nil, picks the first photo in the album by sort_order.
func (s *AlbumService) coverURL(albumID int64, coverID *int64) string {
	if coverID == nil {
		var ap model.AlbumPhoto
		if err := s.DB.Where("album_id = ?", albumID).
			Order("sort_order").First(&ap).Error; err == nil {
			coverID = &ap.PhotoID
		}
	}
	if coverID == nil {
		return ""
	}
	return "/api/thumbnails/" + strconv.FormatInt(*coverID, 10) + ".webp"
}
