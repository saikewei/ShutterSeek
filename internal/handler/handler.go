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
	ttlAlbums      = 60 * time.Second
	ttlDates       = 5 * time.Minute
)

// cstZone is Asia/Shanghai (UTC+8, no DST). Photo dates are stored as UTC in
// the DB but displayed and grouped in local (+08) time; all date parsing and
// formatting must use this zone so jumps, cursors and displays agree.
var cstZone = time.FixedZone("CST", 8*3600)

// Handler holds shared dependencies for all HTTP handlers.
type Handler struct {
	Pool      *pgxpool.Pool
	Redis     *goredis.Client
	DB        *gorm.DB
	OrigSvc   *service.OriginalService
	AlbumSvc  *service.AlbumService
	AuthSvc   *service.AuthService
	SearchSvc *service.SearchService
	UploadSvc *service.UploadService
	PhotoSvc  *service.PhotoService
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
	role := c.GetString("role")
	var albumID int64
	if s := c.Query("album_id"); s != "" {
		if id, err := strconv.ParseInt(s, 10, 64); err == nil && id > 0 {
			albumID = id
		}
	}
	rows, err := h.PhotoSvc.PhotoDates(c.Request.Context(), role, albumID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
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
	params := service.PhotoListParams{
		Limit:      limit,
		Role:       c.GetString("role"),
		Cursor:     c.Query("cursor"),
		AlbumID:    c.Query("album_id"),
		Month:      c.Query("month"),
		Date:       c.Query("date"),
		NewerT:     c.Query("newer_t"),
		NewerID:    mustParseInt64(c.Query("newer_id")),
		WithAlbums: c.Query("with_albums") == "true",
	}
	res, err := h.PhotoSvc.ListPhotos(c.Request.Context(), params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}
	items := make([]PhotoItem, 0, len(res.HeadPhotos)+len(res.Photos))
	for i := len(res.HeadPhotos) - 1; i >= 0; i-- {
		items = append(items, toPhotoItem(&res.HeadPhotos[i]))
	}
	for i := range res.Photos {
		it := toPhotoItem(&res.Photos[i])
		if res.AlbumIDs != nil {
			it.AlbumIDs = res.AlbumIDs[res.Photos[i].ID]
		}
		items = append(items, it)
	}
	c.JSON(http.StatusOK, PhotoListResponse{
		Items:      items,
		Total:      res.Total,
		NextCursor: res.NextCursor,
		HeadCount:  len(res.HeadPhotos),
	})
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

	// Guests may only fetch originals of photos in public albums
	if c.GetString("role") == "guest" {
		var count int64
		h.DB.Raw(
			"SELECT COUNT(*) FROM album_photos ap JOIN albums a ON a.id = ap.album_id WHERE ap.photo_id = ? AND a.is_public = true",
			id,
		).Scan(&count)
		if count == 0 {
			c.JSON(http.StatusForbidden, gin.H{"error": "无权访问"})
			return
		}
	}

	c.Header("Content-Type", "image/jpeg")
	c.Header("Cache-Control", "public, max-age=86400")
	if err := h.OrigSvc.ServeOriginal(c.Writer, photo.FilePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

// ── Redis ───────────────────────────────────────────────

// clearFirstPageCache removes all cached first-page photo responses.
// Call after mutations that change photo visibility (e.g. is_public toggle).
func (h *Handler) clearFirstPageCache() {
	if h.Redis == nil {
		return
	}
	ctx := context.Background()
	var cursor uint64
	for {
		keys, next, err := h.Redis.Scan(ctx, cursor, "cache:first_page:*", 100).Result()
		if err != nil {
			return
		}
		if len(keys) > 0 {
			h.Redis.Del(ctx, keys...)
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
}

// clearAllAlbumCaches removes every cache key that could be affected by an
// album write (list, dates, first page, album photos — all role scopes).
// Album writes are infrequent, so clearing broadly is cheap and safe.
func (h *Handler) clearAllAlbumCaches() {
	if h.Redis == nil {
		return
	}
	ctx := context.Background()
	patterns := []string{
		"cache:albums*",
		"cache:photo_dates*",
		"cache:album_dates*",
		"cache:first_page*",
		"cache:album_photos*",
	}
	for _, pat := range patterns {
		var cursor uint64
		for {
			keys, next, err := h.Redis.Scan(ctx, cursor, pat, 100).Result()
			if err != nil {
				break
			}
			if len(keys) > 0 {
				h.Redis.Del(ctx, keys...)
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
}

// redisSetJSON stores a value as JSON in Redis (no-op when Redis is nil).
func (h *Handler) redisSetJSON(key string, v interface{}, ttl time.Duration) {
	if h.Redis == nil {
		return
	}
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.Redis.Set(context.Background(), key, data, ttl)
}

// redisGetJSON reads a JSON value from Redis into dst. Returns false on
// miss or error (also when Redis is nil).
func (h *Handler) redisGetJSON(key string, dst interface{}) bool {
	if h.Redis == nil {
		return false
	}
	data, err := h.Redis.Get(context.Background(), key).Bytes()
	if err != nil {
		return false
	}
	return json.Unmarshal(data, dst) == nil
}

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

// ── Guest visibility helpers ────────────────────────────

// guestPhotoFilter restricts a photo query to photos that belong to at
// least one public album. Guests see only those photos; admins see all.
func (h *Handler) guestPhotoFilter(c *gin.Context, q *gorm.DB) *gorm.DB {
	if c.GetString("role") == "guest" {
		q = q.Where("id IN (SELECT ap.photo_id FROM album_photos ap JOIN albums a ON a.id = ap.album_id WHERE a.is_public = true)")
	}
	return q
}

// ── helpers ─────────────────────────────────────────────

func split2(s, sep string) []string {
	idx := strings.LastIndex(s, sep)
	if idx < 0 {
		return nil
	}
	return []string{s[:idx], s[idx+1:]}
}

func mustParseInt64(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
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
	// Format in +08 local time so the displayed date matches the date
	// distribution (GROUP BY uses the DB session zone, Asia/Shanghai).
	return t.In(cstZone).Format("2006-01-02T15:04:05")
}
