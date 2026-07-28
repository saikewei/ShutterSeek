// Package handler provides HTTP handlers for the ShutterSeek API.
// Each handler method is attached to Handler which holds shared
// dependencies (database, cache) injected at startup.
package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"shutterseek/internal/model"
)

// Handler holds shared dependencies for all HTTP handlers.
// All fields are injected by main.go at startup.
type Handler struct {
	Pool  *pgxpool.Pool  // pgx native pool (used by legacy code)
	Redis *redis.Client  // Redis client (may be nil if unavailable)
	DB    *gorm.DB       // GORM database handle for ORM queries
}

// ── Health ──────────────────────────────────────────────

// Health returns a simple health check.
// GET /api/health
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ── Photo List (paginated) ──────────────────────────────

// PhotoItem is the JSON response for a single photo in the list view.
// Uses omitempty to keep the response compact for photos without EXIF.
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
// NextCursor is empty string when there are no more pages.
type PhotoListResponse struct {
	Items      []PhotoItem `json:"items"`
	NextCursor string      `json:"next_cursor"` // "" means last page
	Total      int64       `json:"total"`       // total count in database
}

// ListPhotos returns photos sorted by shooting time (newest first).
//
// Cursor-based pagination suitable for infinite scroll:
//
//	GET /api/v1/photos?limit=50
//	GET /api/v1/photos?limit=50&cursor=2023-06-15T14:30:00,1234
//
// The cursor is a composite key: "<taken_at>,<id>".
// Photos without EXIF (taken_at IS NULL) sort last and use cursor
// "0001-01-01T00:00:00,<id>" — paginated by id DESC only.
//
// limit: 1–200, default 50. One extra row is fetched to detect hasMore.
func (h *Handler) ListPhotos(c *gin.Context) {
	// ── parse limit ───────────────────────────────────
	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	// ── parse cursor ───────────────────────────────────
	// Cursor format: "2006-01-02T15:04:05,<id>" or "0001-01-01T00:00:00,<id>" for NULL-time
	var (
		afterTime time.Time
		afterID   int64
	)
	if cur := c.Query("cursor"); cur != "" {
		parts := split2(cur, ",")
		if len(parts) == 2 {
			if t, err := time.Parse("2006-01-02T15:04:05", parts[0]); err == nil && !t.IsZero() {
				afterTime = t
			}
			if n, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				afterID = n
			}
		}
	}

	// ── query ──────────────────────────────────────────
	// Fetch limit+1 rows so we can detect hasMore without a second query.
	var photos []model.Photo
	q := h.DB.Order("taken_at DESC, id DESC").Limit(limit + 1)

	if !afterTime.IsZero() {
		// Normal cursor: (taken_at, id) < (cursor_time, cursor_id)
		// PostgreSQL row-comparison is NULL-safe here because taken_at
		// is never NULL for this branch.
		q = q.Where("(taken_at, id) < (?, ?)", afterTime, afterID)
	} else if afterID > 0 {
		// NULL-time cursor: only photos without EXIF, paginated by id.
		// These appear at the end of the timeline because NULLs sort last.
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
		Total:      h.totalPhotoCount(),
		NextCursor: "",
	}

	// Build the cursor for the next page.
	// For NULL-time photos, use zero-time sentinel so the client can
	// pass it back; we detect it on the next request via isZero.
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		t := last.TakenAt
		if t == "" {
			t = "0001-01-01T00:00:00"
		}
		resp.NextCursor = t + "," + strconv.FormatInt(last.ID, 10)
	}

	c.JSON(http.StatusOK, resp)
}

// ── helpers ─────────────────────────────────────────────

// split2 splits s at the last occurrence of sep.
// Used to parse "time,id" cursors where time may contain '-' and 'T'.
func split2(s, sep string) []string {
	idx := strings.LastIndex(s, sep)
	if idx < 0 {
		return nil
	}
	return []string{s[:idx], s[idx+1:]}
}

// totalPhotoCount returns the total number of photos (cached in future).
func (h *Handler) totalPhotoCount() int64 {
	var count int64
	h.DB.Model(&model.Photo{}).Count(&count)
	return count
}

// formatFocal returns "27mm" or "" for zero.
func formatFocal(f float64) string {
	if f == 0 {
		return ""
	}
	return strconv.FormatFloat(f, 'f', 0, 64) + "mm"
}

// formatAperture returns "ƒ/2.8" or "" for zero.
func formatAperture(f float64) string {
	if f == 0 {
		return ""
	}
	return "ƒ/" + strconv.FormatFloat(f, 'f', 1, 64)
}

// formatTime returns "2006-01-02 15:04:05" or "" for zero time.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}
