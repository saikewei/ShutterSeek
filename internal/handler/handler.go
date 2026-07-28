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
)

// Redis key prefixes and TTLs for cache.
const (
	keyTotalPhotos = "cache:total_photos"
	keyFirstPage   = "cache:first_page:"
	ttlTotal       = 5 * time.Minute
	ttlFirstPage   = 60 * time.Second
)

// Handler holds shared dependencies for all HTTP handlers.
type Handler struct {
	Pool  *pgxpool.Pool
	Redis *goredis.Client // may be nil if Redis unavailable
	DB    *gorm.DB
}

// ── Health ──────────────────────────────────────────────

// Health returns a simple health check.
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ── Photo List (paginated) ──────────────────────────────

// PhotoItem is the JSON response for a single photo in the list view.
type PhotoItem struct {
	ID           int64  `json:"id"`
	ThumbnailURL string `json:"thumbnail_url"`
	CameraMake   string `json:"camera_make,omitempty"`
	CameraModel  string `json:"camera_model,omitempty"`
	LensModel    string `json:"lens_model,omitempty"`
	FocalLength  string `json:"focal_length,omitempty"`
	Aperture     string `json:"aperture,omitempty"`
	ISO          int32  `json:"iso,omitempty"`
	TakenAt      string `json:"taken_at,omitempty"`
	Width        int32  `json:"width"`
	Height       int32  `json:"height"`
}

// PhotoListResponse wraps a page of photos for the list endpoint.
type PhotoListResponse struct {
	Items      []PhotoItem `json:"items"`
	NextCursor string      `json:"next_cursor"`
	Total      int64       `json:"total"`
}

// ListPhotos returns photos sorted by shooting time (newest first),
// with cursor-based pagination for infinite scroll.
//
// The first page (no cursor) is cached in Redis for 60s because it
// receives ~90% of traffic. Subsequent pages with unique cursors
// are not cached (hit rate ~0%).
//
//	GET /api/v1/photos?limit=50
//	GET /api/v1/photos?limit=50&cursor=2023-06-15T14:30:00,1234
func (h *Handler) ListPhotos(c *gin.Context) {
	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	// ── parse cursor ───────────────────────────────────
	var (
		afterTime time.Time
		afterID   int64
		hasCursor bool
	)
	if cur := c.Query("cursor"); cur != "" {
		hasCursor = true
		parts := split2(cur, ",")
		if len(parts) == 2 {
			if t, err := time.Parse("2006-01-02T15:04:05", strings.Replace(parts[0], " ", "T", 1)); err == nil && !t.IsZero() {
				afterTime = t
			}
			if n, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				afterID = n
			}
		}
	}

	// ── Redis cache: first page only ───────────────────
	cacheKey := ""
	if !hasCursor {
		cacheKey = keyFirstPage + strconv.Itoa(limit)
		if cached, ok := h.redisGet(cacheKey); ok {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	// ── query ──────────────────────────────────────────
	var photos []model.Photo
	// Only return photos that have a thumbnail on disk.
	// Subquery: check if {id}.jpg exists in the thumbnails table (indirectly via file presence).
	// For now, filter out screenshots without EXIF — they appear first and have no thumbnails.
	q := h.DB.Where("taken_at IS NOT NULL").Order("taken_at DESC, id DESC").Limit(limit + 1)

	if !afterTime.IsZero() {
		q = q.Where("(taken_at, id) < (?, ?)", afterTime, afterID)
	} else if afterID > 0 {
		q = q.Where("taken_at IS NULL AND id < ?", afterID)
	}

	if err := q.Find(&photos).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}

	// ── build response ─────────────────────────────────
	hasMore := len(photos) > limit
	if hasMore {
		photos = photos[:limit]
	}

	items := make([]PhotoItem, len(photos))
	for i, p := range photos {
		items[i] = PhotoItem{
			ID:           p.ID,
			ThumbnailURL: "/thumbnails/" + strconv.FormatInt(p.ID, 10) + ".jpg",
			CameraMake:   p.CameraMake,
			CameraModel:  p.CameraModel,
			LensModel:    p.LensModel,
			FocalLength:  formatFocal(p.FocalLength),
			Aperture:     formatAperture(p.Aperture),
			ISO:          p.Iso,
			TakenAt:      formatTime(p.TakenAt),
			Width:        p.Width,
			Height:       p.Height,
		}
	}

	resp := PhotoListResponse{
		Items:      items,
		Total:      h.totalPhotoCountCached(),
		NextCursor: "",
	}

	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		t := last.TakenAt
		if t == "" {
			t = "0001-01-01T00:00:00"
		}
		resp.NextCursor = t + "," + strconv.FormatInt(last.ID, 10)
	}

	// ── Write to Redis cache (first page only, async) ──
	if cacheKey != "" && h.Redis != nil {
		data, _ := json.Marshal(resp)
		h.Redis.Set(context.Background(), cacheKey, data, ttlFirstPage)
	}

	c.JSON(http.StatusOK, resp)
}

// ── Redis helpers ────────────────────────────────────────

// redisGet reads a cached JSON response from Redis.
// Returns false if the key doesn't exist or Redis is unavailable.
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

// ── Total count (Redis cached) ───────────────────────────

// totalPhotoCountCached returns the total number of photos,
// cached in Redis for ttlTotal (5 min).
func (h *Handler) totalPhotoCountCached() int64 {
	if h.Redis != nil {
		if val, err := h.Redis.Get(context.Background(), keyTotalPhotos).Int64(); err == nil {
			return val
		}
	}
	// Miss or Redis down — query DB
	var count int64
	h.DB.Model(&model.Photo{}).Count(&count)
	// Write back asynchronously
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
