// Package handler provides HTTP handlers for the ShutterSeek API.
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"shutterseek/internal/model"
	"shutterseek/internal/service"
)

const (
	keyTotalPhotos = "cache:total_photos"
	keyFirstPage   = "cache:first_page:"
	ttlTotal       = 5 * time.Minute
	ttlFirstPage   = 60 * time.Second
)

// Handler holds shared dependencies for all HTTP handlers.
type Handler struct {
	Pool     *pgxpool.Pool
	Redis    *goredis.Client
	DB       *gorm.DB
	OrigSvc  *service.OriginalService
	AlbumSvc *service.AlbumService
}

// ── Health ──────────────────────────────────────────────

func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ── Photo Dates ──────────────────────────────────────────

type DateCount struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

// PhotoDates returns date distribution for all photos.
// GET /api/v1/photos/dates
func (h *Handler) PhotoDates(c *gin.Context) {
	var rows []DateCount
	query := "SELECT to_char(p.taken_at, 'YYYY-MM-DD') AS date, COUNT(*) AS count FROM photos p"
	args := []interface{}{}

	if albumIDStr := c.Query("album_id"); albumIDStr != "" {
		if albumID, err := strconv.ParseInt(albumIDStr, 10, 64); err == nil && albumID > 0 {
			query += " JOIN album_photos ap ON ap.photo_id = p.id WHERE ap.album_id = ? AND p.taken_at IS NOT NULL"
			args = append(args, albumID)
		} else {
			query += " WHERE p.taken_at IS NOT NULL"
		}
	} else {
		query += " WHERE p.taken_at IS NOT NULL"
	}
	query += " GROUP BY date ORDER BY date DESC"

	if err := h.DB.Raw(query, args...).Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}
	if rows == nil {
		rows = []DateCount{}
	}
	c.JSON(http.StatusOK, rows)
}

// ── Photo List ──────────────────────────────────────────

type PhotoItem struct {
	ID           int64   `json:"id"`
	ThumbnailURL string  `json:"thumbnail_url"`
	FileName     string  `json:"file_name,omitempty"`
	CameraMake   string  `json:"camera_make,omitempty"`
	CameraModel  string  `json:"camera_model,omitempty"`
	LensModel    string  `json:"lens_model,omitempty"`
	FocalLength  string  `json:"focal_length,omitempty"`
	Aperture     string  `json:"aperture,omitempty"`
	ISO          int32   `json:"iso,omitempty"`
	TakenAt      string  `json:"taken_at,omitempty"`
	Width        int32   `json:"width"`
	Height       int32   `json:"height"`
	FilePath     string  `json:"file_path,omitempty"`
	AlbumIDs     []int64 `json:"album_ids,omitempty"`
}

type PhotoListResponse struct {
	Items      []PhotoItem `json:"items"`
	NextCursor string      `json:"next_cursor"`
	Total      int64       `json:"total"`
	HeadCount  int         `json:"head_count,omitempty"`
}

// ListPhotos returns photos sorted by shooting time (newest first),
// cursor-based pagination for infinite scroll.
func (h *Handler) ListPhotos(c *gin.Context) {
	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	var (
		afterTime time.Time
		afterID   int64
		hasCursor bool
	)
	if cur := c.Query("cursor"); cur != "" {
		hasCursor = true
		parts := split2(cur, ",")
		if len(parts) == 2 {
			ts := strings.Replace(parts[0], " ", "T", 1)
			if t, err := time.Parse("2006-01-02T15:04:05", ts); err == nil && !t.IsZero() {
				afterTime = t
			}
			if n, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				afterID = n
			}
		}
	}

	// Redis cache for first page only (skip when filtering)
	cacheKey := ""
	uncategorized := c.Query("album_id") == "none"
	month := c.Query("month")
	albumIDCache := c.Query("album_id")
	if albumIDCache == "none" {
		albumIDCache = ""
	}
	if !hasCursor && !uncategorized && month == "" {
		if albumIDCache != "" {
			cacheKey = keyFirstPage + "album:" + albumIDCache + ":" + strconv.Itoa(limit)
		} else {
			cacheKey = keyFirstPage + strconv.Itoa(limit)
		}
		if cached, ok := h.redisGet(cacheKey); ok {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	var photos []model.Photo
	q := h.DB.Where("taken_at IS NOT NULL")

	// Filter: specific album
	albumIDStr := c.Query("album_id")
	if albumIDStr != "" {
		if albumID, err := strconv.ParseInt(albumIDStr, 10, 64); err == nil && albumID > 0 {
			q = q.Where("id IN (SELECT photo_id FROM album_photos WHERE album_id = ?)", albumID)
		}
	}

	// Reverse pagination: load newer photos
	newerT := c.Query("newer_t")
	if newerT != "" {
		if t, err := time.Parse("2006-01-02T15:04:05", newerT); err == nil && !t.IsZero() {
			if newerID, err2 := strconv.ParseInt(c.Query("newer_id"), 10, 64); err2 == nil {
				q = q.Where("(taken_at, id) > (?, ?)", t, newerID)
			}
		}
		q = q.Order("taken_at ASC, id ASC").Limit(limit + 1)
	} else {
		q = q.Order("taken_at DESC, id DESC").Limit(limit + 1)
	}

// Filter: jump to month — with head preload
		var headPhotos []model.Photo
		if month != "" {
			if t, err := time.Parse("2006-01", month); err == nil {
				nextMonth := t.AddDate(0, 1, 0)
				// Preload a few photos from the next month (newer) as head
				headQ := h.DB.Where("taken_at >= ?", nextMonth)
				if albumIDStr != "" {
					if aid, err2 := strconv.ParseInt(albumIDStr, 10, 64); err2 == nil && aid > 0 {
						headQ = headQ.Where("id IN (SELECT photo_id FROM album_photos WHERE album_id = ?)", aid)
					}
				}
				headQ.Order("taken_at ASC, id ASC").Limit(15).Find(&headPhotos)
				// Main query: target month and older
				q = q.Where("taken_at < ?", nextMonth)
			}
		}

	// Filter: uncategorized only
	if uncategorized {
		q = q.Where("id NOT IN (SELECT DISTINCT photo_id FROM album_photos)")
	}

	if !afterTime.IsZero() {
		q = q.Where("(taken_at, id) < (?, ?)", afterTime, afterID)
	} else if afterID > 0 {
		q = q.Where("taken_at IS NULL AND id < ?", afterID)
	}

	if err := q.Find(&photos).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}

	hasMore := len(photos) > limit
	if hasMore {
		photos = photos[:limit]
	}

	// Prepend head photos (reverse to DESC order)
	allPhotos := make([]model.Photo, 0, len(headPhotos)+len(photos))
	for i := len(headPhotos) - 1; i >= 0; i-- {
		allPhotos = append(allPhotos, headPhotos[i])
	}
	allPhotos = append(allPhotos, photos...)

	// Load album IDs if requested
	withAlbums := c.Query("with_albums") == "true"
	var albumMap map[int64][]int64
	if withAlbums && len(allPhotos) > 0 {
		ids := make([]int64, len(allPhotos))
		for i, p := range allPhotos {
			ids[i] = p.ID
		}
		albumMap, _ = h.AlbumSvc.GetPhotoAlbumIDs(ids)
	}

	items := make([]PhotoItem, len(allPhotos))
	for i, p := range allPhotos {
		items[i] = toPhotoItem(&p)
		if albumMap != nil {
			items[i].AlbumIDs = albumMap[p.ID]
		}
	}

	var total int64
	switch {
	case uncategorized:
		h.DB.Model(&model.Photo{}).Where("taken_at IS NOT NULL AND id NOT IN (SELECT DISTINCT photo_id FROM album_photos)").Count(&total)
	case albumIDStr != "":
		if albumID, err := strconv.ParseInt(albumIDStr, 10, 64); err == nil && albumID > 0 {
			tq := h.DB.Model(&model.Photo{}).
				Where("taken_at IS NOT NULL AND id IN (SELECT photo_id FROM album_photos WHERE album_id = ?)", albumID)
			if month != "" {
				if t, err := time.Parse("2006-01", month); err == nil {
					tq = tq.Where("taken_at < ?", t.AddDate(0, 1, 0))
				}
			}
			tq.Count(&total)
		}
	default:
		total = h.totalPhotoCountCached()
	}

	resp := PhotoListResponse{
		Items:      items,
		Total:      total,
		NextCursor: "",
		HeadCount:  len(headPhotos),
	}

	if hasMore && len(photos) > 0 {
		lastItem := items[len(items)-1]
		t := lastItem.TakenAt
		if t == "" {
			t = "0001-01-01T00:00:00"
		}
		resp.NextCursor = t + "," + strconv.FormatInt(lastItem.ID, 10)
	}

	if cacheKey != "" && h.Redis != nil {
		data, _ := json.Marshal(resp)
		h.Redis.Set(context.Background(), cacheKey, data, ttlFirstPage)
	}

	c.JSON(http.StatusOK, resp)
}

// ── Original Photo ───────────────────────────────────────

// GetOriginal serves the original photo file as JPEG.
// RAW files have their embedded preview extracted; TIFFs are decoded and re-encoded.
// GET /api/v1/photos/:id/original
func (h *Handler) GetOriginal(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var photo model.Photo
	if err := h.DB.Where("id = ?", id).First(&photo).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "photo not found"})
		return
	}

	c.Header("Content-Type", "image/jpeg")
	c.Header("Cache-Control", "public, max-age=86400")
	if err := h.OrigSvc.ServeOriginal(c.Writer, photo.FilePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

// ── Redis ───────────────────────────────────────────────

func (h *Handler) redisGet(key string) (PhotoListResponse, bool) {
	if h.Redis == nil {
		return PhotoListResponse{}, false
	}
	data, err := h.Redis.Get(context.Background(), key).Bytes()
	if err != nil {
		return PhotoListResponse{}, false
	}
	var resp PhotoListResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return PhotoListResponse{}, false
	}
	return resp, true
}

func (h *Handler) totalPhotoCountCached() int64 {
	if h.Redis != nil {
		if val, err := h.Redis.Get(context.Background(), keyTotalPhotos).Int64(); err == nil {
			return val
		}
	}
	var count int64
	h.DB.Model(&model.Photo{}).Count(&count)
	if h.Redis != nil {
		h.Redis.Set(context.Background(), keyTotalPhotos, count, ttlTotal)
	}
	return count
}

// ── helpers ─────────────────────────────────────────────

func split2(s, sep string) []string {
	idx := strings.LastIndex(s, sep)
	if idx < 0 {
		return nil
	}
	return []string{s[:idx], s[idx+1:]}
}

func formatFocal(f float64) string {
	if f == 0 {
		return ""
	}
	return strconv.FormatFloat(f, 'f', 0, 64) + "mm"
}

func formatAperture(f float64) string {
	if f == 0 {
		return ""
	}
	return "ƒ/" + strconv.FormatFloat(f, 'f', 1, 64)
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02T15:04:05")
}
