package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"shutterseek/internal/model"
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

type PhotoListParams struct {
	Limit      int
	Role       string
	Cursor     string
	AlbumID    string // "none" = uncategorized
	Month      string
	Date       string
	NewerT     string
	NewerID    int64
	WithAlbums bool
}

type PhotoListResult struct {
	Photos     []model.Photo
	HeadPhotos []model.Photo
	Total      int64
	HasMore    bool
	NextCursor string
	AlbumIDs   map[int64][]int64
}

func split2(s, sep string) []string {
	idx := strings.LastIndex(s, sep)
	if idx < 0 {
		return nil
	}
	return []string{s[:idx], s[idx+1:]}
}

func parseCursor(s string) (time.Time, int64, bool) {
	parts := split2(s, ",")
	if len(parts) != 2 {
		return time.Time{}, 0, false
	}
	ts := strings.Replace(parts[0], " ", "T", 1)
	t, err := time.ParseInLocation("2006-01-02T15:04:05", ts, cstZone)
	if err != nil || t.IsZero() {
		return time.Time{}, 0, false
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return time.Time{}, 0, false
	}
	return t, id, true
}

func buildNextCursor(t time.Time, id int64) string {
	if t.IsZero() {
		t = time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return t.Format("2006-01-02T15:04:05") + "," + strconv.FormatInt(id, 10)
}

// guestFilter 给查询加上 guest 可见性条件。
func (s *PhotoService) guestFilter(q *gorm.DB, role string) *gorm.DB {
	if role == "guest" {
		q = q.Where("id IN (SELECT ap.photo_id FROM album_photos ap JOIN albums a ON a.id = ap.album_id WHERE a.is_public = true)")
	}
	return q
}

// ListPhotos 返回按拍摄时间倒序的照片分页（含首页缓存、guest 作用域、日期/月份跳转与 head 预载）。
// SQL 与语义与重构前 handler 逐字一致。
func (s *PhotoService) ListPhotos(ctx context.Context, p PhotoListParams) (*PhotoListResult, error) {
	limit := p.Limit
	if limit <= 0 {
		limit = 50
	}
	var (
		afterTime time.Time
		afterID   int64
		hasCursor bool
	)
	if p.Cursor != "" {
		hasCursor = true
		afterTime, afterID, _ = parseCursor(p.Cursor)
	}

	roleScope := ""
	if p.Role == "guest" {
		roleScope = "guest:"
	}
	cacheKey := ""
	uncategorized := p.AlbumID == "none"
	albumIDCache := p.AlbumID
	if albumIDCache == "none" {
		albumIDCache = ""
	}
	if !hasCursor && !uncategorized && p.Month == "" && p.Date == "" && p.NewerT == "" {
		if albumIDCache != "" {
			cacheKey = KeyFirstPage + roleScope + "album:" + albumIDCache + ":" + strconv.Itoa(limit)
		} else {
			cacheKey = KeyFirstPage + roleScope + strconv.Itoa(limit)
		}
		if s.Cache != nil {
			if data, ok := s.Cache.GetBytes(cacheKey); ok {
				var cached PhotoListResult
				if json.Unmarshal(data, &cached) == nil {
					return &cached, nil
				}
			}
		}
	}

	var photos []model.Photo
	q := s.DB.WithContext(ctx).Where("taken_at IS NOT NULL")
	q = s.guestFilter(q, p.Role)

	albumIDStr := p.AlbumID
	if albumIDStr != "" && albumIDStr != "none" {
		if albumID, err := strconv.ParseInt(albumIDStr, 10, 64); err == nil && albumID > 0 {
			q = q.Where("id IN (SELECT photo_id FROM album_photos WHERE album_id = ?)", albumID)
		}
	}

	if p.NewerT != "" {
		if t, err := time.ParseInLocation("2006-01-02T15:04:05", p.NewerT, cstZone); err == nil && !t.IsZero() {
			q = q.Where("(taken_at, id) > (?, ?)", t, p.NewerID)
		}
		q = q.Order("taken_at ASC, id ASC").Limit(limit + 1)
	} else {
		q = q.Order("taken_at DESC, id DESC").Limit(limit + 1)
	}

	var headPhotos []model.Photo
	if p.Date != "" {
		if t, err := time.ParseInLocation("2006-01-02", p.Date, cstZone); err == nil {
			nextDay := t.AddDate(0, 0, 1)
			headQ := s.DB.WithContext(ctx).Where("taken_at >= ?", nextDay)
			headQ = s.guestFilter(headQ, p.Role)
			if albumIDStr != "" && albumIDStr != "none" {
				if aid, err2 := strconv.ParseInt(albumIDStr, 10, 64); err2 == nil && aid > 0 {
					headQ = headQ.Where("id IN (SELECT photo_id FROM album_photos WHERE album_id = ?)", aid)
				}
			}
			headQ.Order("taken_at ASC, id ASC").Limit(15).Find(&headPhotos)
			q = q.Where("taken_at < ?", nextDay)
		}
	} else if p.Month != "" {
		if t, err := time.ParseInLocation("2006-01", p.Month, cstZone); err == nil {
			nextMonth := t.AddDate(0, 1, 0)
			headQ := s.DB.WithContext(ctx).Where("taken_at >= ?", nextMonth)
			headQ = s.guestFilter(headQ, p.Role)
			if albumIDStr != "" && albumIDStr != "none" {
				if aid, err2 := strconv.ParseInt(albumIDStr, 10, 64); err2 == nil && aid > 0 {
					headQ = headQ.Where("id IN (SELECT photo_id FROM album_photos WHERE album_id = ?)", aid)
				}
			}
			headQ.Order("taken_at ASC, id ASC").Limit(15).Find(&headPhotos)
			q = q.Where("taken_at < ?", nextMonth)
		}
	}

	if uncategorized {
		q = q.Where("id NOT IN (SELECT DISTINCT photo_id FROM album_photos)")
	}

	if !afterTime.IsZero() {
		q = q.Where("(taken_at, id) < (?, ?)", afterTime, afterID)
	} else if afterID > 0 {
		q = q.Where("taken_at IS NULL AND id < ?", afterID)
	}

	if err := q.Find(&photos).Error; err != nil {
		return nil, err
	}

	hasMore := len(photos) > limit
	if hasMore {
		photos = photos[:limit]
	}

	allPhotos := make([]model.Photo, 0, len(headPhotos)+len(photos))
	for i := len(headPhotos) - 1; i >= 0; i-- {
		allPhotos = append(allPhotos, headPhotos[i])
	}
	allPhotos = append(allPhotos, photos...)

	var albumMap map[int64][]int64
	if p.WithAlbums && len(allPhotos) > 0 {
		ids := make([]int64, len(allPhotos))
		for i, ph := range allPhotos {
			ids[i] = ph.ID
		}
		albumMap, _ = s.AlbumSvc.GetPhotoAlbumIDs(ids)
	}

	var boundary *time.Time
	if p.Date != "" {
		if t, err := time.ParseInLocation("2006-01-02", p.Date, cstZone); err == nil {
			b := t.AddDate(0, 0, 1)
			boundary = &b
		}
	} else if p.Month != "" {
		if t, err := time.ParseInLocation("2006-01", p.Month, cstZone); err == nil {
			b := t.AddDate(0, 1, 0)
			boundary = &b
		}
	}

	var total int64
	switch {
	case roleScope != "": // guest — count only public-album photos
		tq := s.DB.Model(&model.Photo{}).
			Where("taken_at IS NOT NULL AND id IN (SELECT ap.photo_id FROM album_photos ap JOIN albums a ON a.id = ap.album_id WHERE a.is_public = true)")
		if albumIDStr != "" && albumIDStr != "none" {
			if albumID, err := strconv.ParseInt(albumIDStr, 10, 64); err == nil && albumID > 0 {
				tq = tq.Where("id IN (SELECT photo_id FROM album_photos WHERE album_id = ?)", albumID)
			}
		}
		if boundary != nil {
			tq = tq.Where("taken_at < ?", *boundary)
		}
		tq.Count(&total)
	case uncategorized:
		s.DB.Model(&model.Photo{}).Where("taken_at IS NOT NULL AND id NOT IN (SELECT DISTINCT photo_id FROM album_photos)").Count(&total)
	case albumIDStr != "" && albumIDStr != "none":
		if albumID, err := strconv.ParseInt(albumIDStr, 10, 64); err == nil && albumID > 0 {
			tq := s.DB.Model(&model.Photo{}).
				Where("taken_at IS NOT NULL AND id IN (SELECT photo_id FROM album_photos WHERE album_id = ?)", albumID)
			if boundary != nil {
				tq = tq.Where("taken_at < ?", *boundary)
			}
			tq.Count(&total)
		}
	default:
		if boundary != nil {
			s.DB.Model(&model.Photo{}).Where("taken_at IS NOT NULL AND taken_at < ?", *boundary).Count(&total)
		} else {
			total = s.TotalPhotoCountCached(ctx)
		}
	}

	res := &PhotoListResult{
		Photos:     photos,
		HeadPhotos: headPhotos,
		Total:      total,
		HasMore:    hasMore,
		AlbumIDs:   albumMap,
	}
	if hasMore && len(photos) > 0 {
		last := photos[len(photos)-1]
		res.NextCursor = buildNextCursor(last.TakenAt, last.ID)
	}

	if cacheKey != "" && s.Cache != nil {
		if data, err := json.Marshal(res); err == nil {
			s.Cache.Redis.Set(ctx, cacheKey, data, TTLFirstPage)
		}
	}
	return res, nil
}

func (s *PhotoService) TotalPhotoCountCached(ctx context.Context) int64 {
	if s.Cache != nil {
		if data, ok := s.Cache.GetBytes(KeyTotalPhotos); ok {
			if n, err := strconv.ParseInt(string(data), 10, 64); err == nil {
				return n
			}
		}
	}
	var count int64
	s.DB.Model(&model.Photo{}).Count(&count)
	if s.Cache != nil {
		s.Cache.Redis.Set(ctx, KeyTotalPhotos, count, TTLTotal)
	}
	return count
}
