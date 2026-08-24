// Package handler provides HTTP handlers for the ShutterSeek API.
package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"shutterseek/internal/service"
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
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	photo, err := h.PhotoSvc.GetPhoto(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "photo not found"})
		return
	}
	if c.GetString("role") == "guest" {
		ok, err := h.PhotoSvc.PhotoInPublicAlbum(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
			return
		}
		if !ok {
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

// ── helpers ─────────────────────────────────────────────

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
