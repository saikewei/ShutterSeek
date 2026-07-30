package service

import (
	"errors"
	"strconv"
	"time"

	"gorm.io/gorm"

	"shutterseek/internal/model"
)

// Sentinel errors for album service.
var (
	ErrAlbumNotFound  = errors.New("album not found")
	ErrPhotoNotInAlbum = errors.New("photo not in album")
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

// ── Mutations ─────────────────────────────────────────

// CreateAlbum creates a new album.
func (s *AlbumService) CreateAlbum(title, description string) (*AlbumItem, error) {
	a := model.Album{Title: title, Description: description}
	if err := s.DB.Create(&a).Error; err != nil {
		return nil, err
	}
	return s.GetAlbum(a.ID)
}

// UpdateAlbum updates an album's title, description, and/or cover photo.
// Pass nil for fields you don't want to change.
// coverPhotoID: nil = no change, pointer to -1 = clear (auto), pointer to N = set to N.
func (s *AlbumService) UpdateAlbum(id int64, title, description *string, coverPhotoID *int64) (*AlbumItem, error) {
	tx := s.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var a model.Album
	if err := tx.First(&a, id).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAlbumNotFound
		}
		return nil, err
	}

	if title != nil {
		a.Title = *title
	}
	if description != nil {
		a.Description = *description
	}
	if coverPhotoID != nil {
		if *coverPhotoID <= 0 {
			a.CoverPhotoID = nil
		} else {
			var count int64
			tx.Model(&model.AlbumPhoto{}).
				Where("album_id = ? AND photo_id = ?", id, *coverPhotoID).Count(&count)
			if count == 0 {
				tx.Rollback()
				return nil, ErrPhotoNotInAlbum
			}
			a.CoverPhotoID = coverPhotoID
		}
	}
	a.UpdatedAt = time.Now()

	if err := tx.Save(&a).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return s.GetAlbum(id)
}

// DeleteAlbum deletes an album and all its photo associations.
func (s *AlbumService) DeleteAlbum(id int64) error {
	tx := s.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Delete associations first, then the album
	if err := tx.Where("album_id = ?", id).Delete(&model.AlbumPhoto{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Delete(&model.Album{}, id).Error; err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

// RemoveAlbumPhoto removes a single photo from an album.
func (s *AlbumService) RemoveAlbumPhoto(albumID, photoID int64) error {
	tx := s.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	res := tx.Where("album_id = ? AND photo_id = ?", albumID, photoID).
		Delete(&model.AlbumPhoto{})
	if res.Error != nil {
		tx.Rollback()
		return res.Error
	}
	if res.RowsAffected == 0 {
		tx.Rollback()
		return ErrPhotoNotInAlbum
	}

	// If the removed photo was the cover, clear it
	var a model.Album
	if err := tx.First(&a, albumID).Error; err == nil {
		if a.CoverPhotoID != nil && *a.CoverPhotoID == photoID {
			a.CoverPhotoID = nil
			tx.Save(&a)
		}
	}

	return tx.Commit().Error
}

// BatchAddResult reports how many photos were added vs skipped.
type BatchAddResult struct {
	Added   int64
	Skipped int64
}

// BatchAddPhotos adds photos to an album, skipping those that already exist.
func (s *AlbumService) BatchAddPhotos(albumID int64, photoIDs []int64) (*BatchAddResult, error) {
	tx := s.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Verify album exists
	var a model.Album
	if err := tx.First(&a, albumID).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAlbumNotFound
		}
		return nil, err
	}

	var added, skipped int64
	for _, pid := range photoIDs {
		ap := model.AlbumPhoto{AlbumID: albumID, PhotoID: pid, AddedAt: time.Now()}
		res := tx.Where("album_id = ? AND photo_id = ?", albumID, pid).FirstOrCreate(&ap)
		if res.Error != nil {
			tx.Rollback()
			return nil, res.Error
		}
		if res.RowsAffected > 0 {
			added++
		} else {
			skipped++
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return &BatchAddResult{Added: added, Skipped: skipped}, nil
}

// GetPhotoAlbumIDs returns the album IDs each photo belongs to.
func (s *AlbumService) GetPhotoAlbumIDs(photoIDs []int64) (map[int64][]int64, error) {
	if len(photoIDs) == 0 {
		return nil, nil
	}

	var rows []model.AlbumPhoto
	if err := s.DB.Where("photo_id IN ?", photoIDs).Find(&rows).Error; err != nil {
		return nil, err
	}

	result := make(map[int64][]int64)
	for _, r := range rows {
		result[r.PhotoID] = append(result[r.PhotoID], r.AlbumID)
	}
	return result, nil
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
