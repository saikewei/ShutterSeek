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
type Handler struct {
	Pool  *pgxpool.Pool
	Redis *redis.Client
	DB    *gorm.DB
}

// Health returns a simple health check.
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// PhotoItem is the JSON response for one photo in the list.
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

// PhotoListResponse is the paginated response.
type PhotoListResponse struct {
	Items      []PhotoItem `json:"items"`
	NextCursor string      `json:"next_cursor"` // empty if no more pages
	Total      int64       `json:"total"`
}

// ListPhotos returns photos sorted by shooting time (newest first), cursor-based for infinite scroll.
// Query: ?cursor=<taken_at>,<id>&limit=<n> (default limit=50)
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

	var photos []model.Photo
	q := h.DB.Order("taken_at DESC, id DESC").Limit(limit + 1)

	if !afterTime.IsZero() {
		q = q.Where("(taken_at, id) < (?, ?)", afterTime, afterID)
	} else if afterID > 0 {
		// cursor from NULL-time photos — paginate by id only
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
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		// TakenAt may be empty (zero); use zero-time as cursor for those
		t := last.TakenAt
		if t == "" {
			t = "0001-01-01T00:00:00"
		}
		resp.NextCursor = t + "," + strconv.FormatInt(last.ID, 10)
	}

	c.JSON(http.StatusOK, resp)
}

func split2(s, sep string) []string {
	idx := strings.LastIndex(s, sep)
	if idx < 0 {
		return nil
	}
	return []string{s[:idx], s[idx+1:]}
}

func (h *Handler) totalPhotoCount() int64 {
	var count int64
	h.DB.Model(&model.Photo{}).Count(&count)
	return count
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
	return t.Format("2006-01-02 15:04:05")
}
