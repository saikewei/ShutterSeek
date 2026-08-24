package service

import (
	"context"
	"strconv"

	"gorm.io/gorm"
)

type DateCount struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

type PhotoService struct {
	DB       *gorm.DB
	Cache    *Cache
	AlbumSvc *AlbumService
}

func NewPhotoService(db *gorm.DB, cache *Cache, albumSvc *AlbumService) *PhotoService {
	return &PhotoService{DB: db, Cache: cache, AlbumSvc: albumSvc}
}

// PhotoDates 返回日期分布（含 Redis 缓存与 guest 作用域），SQL 与重构前逐字一致。
func (s *PhotoService) PhotoDates(ctx context.Context, role string, albumID int64) ([]DateCount, error) {
	roleScope := ""
	if role == "guest" {
		roleScope = "guest:"
	}
	cacheKey := "cache:photo_dates:" + roleScope
	if albumID > 0 {
		cacheKey += "album:" + strconv.FormatInt(albumID, 10)
	}

	var rows []DateCount
	if s.Cache != nil && s.Cache.GetJSON(cacheKey, &rows) {
		return rows, nil
	}

	query := "SELECT to_char(p.taken_at, 'YYYY-MM-DD') AS date, COUNT(*) AS count FROM photos p"
	args := []interface{}{}

	if albumID > 0 {
		query += " JOIN album_photos ap ON ap.photo_id = p.id WHERE ap.album_id = ? AND p.taken_at IS NOT NULL"
		args = append(args, albumID)
	} else {
		query += " WHERE p.taken_at IS NOT NULL"
	}

	if role == "guest" {
		query += " AND p.id IN (SELECT ap.photo_id FROM album_photos ap JOIN albums a ON a.id = ap.album_id WHERE a.is_public = true)"
	}
	query += " GROUP BY date ORDER BY date DESC"

	if err := s.DB.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []DateCount{}
	}
	if s.Cache != nil {
		s.Cache.SetJSON(cacheKey, rows, TTLDates)
	}
	return rows, nil
}
