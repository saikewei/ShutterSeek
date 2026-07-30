package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"shutterseek/internal/model"
)

// ── Album List ──────────────────────────────────────────

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

// ListAlbums returns all albums.
// GET /api/v1/albums
func (h *Handler) ListAlbums(c *gin.Context) {
	svcItems, err := h.AlbumSvc.ListAlbums()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list albums failed"})
		return
	}

	items := make([]AlbumItem, len(svcItems))
	for i, it := range svcItems {
		items[i] = AlbumItem(it)
	}

	c.JSON(http.StatusOK, gin.H{
		"items": items,
		"total": len(items),
	})
}

// ── Album Detail ────────────────────────────────────────

// GetAlbum returns a single album.
// GET /api/v1/albums/:id
func (h *Handler) GetAlbum(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	item, err := h.AlbumSvc.GetAlbum(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "album not found"})
		return
	}

	c.JSON(http.StatusOK, AlbumItem(*item))
}

// ── Album Photos ────────────────────────────────────────

// AlbumPhotoListResponse is the paginated response for album photos.
type AlbumPhotoListResponse struct {
	Items      []PhotoItem `json:"items"`
	NextCursor string      `json:"next_cursor"`
	Total      int64       `json:"total"`
}

// ListAlbumPhotos returns paginated photos within an album.
// GET /api/v1/albums/:id/photos
func (h *Handler) ListAlbumPhotos(c *gin.Context) {
	albumID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

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
			ts := parts[0]
			if t, err := time.Parse("2006-01-02T15:04:05", ts); err == nil && !t.IsZero() {
				afterTime = t
			}
			if n, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				afterID = n
			}
		}
	}

	// Redis cache for first page
	cacheKey := ""
	if !hasCursor {
		cacheKey = "cache:album_photos:" + strconv.FormatInt(albumID, 10) + ":" + strconv.Itoa(limit)
		if cached, ok := h.redisGet(cacheKey); ok {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	page, err := h.AlbumSvc.ListAlbumPhotos(albumID, limit, afterTime, afterID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}

	items := make([]PhotoItem, len(page.Photos))
	for i, p := range page.Photos {
		items[i] = toPhotoItem(&p)
	}

	resp := AlbumPhotoListResponse{
		Items:      items,
		Total:      page.Total,
		NextCursor: "",
	}

	if page.HasMore && len(items) > 0 {
		last := items[len(items)-1]
		t := last.TakenAt
		if t == "" {
			t = "0001-01-01T00:00:00"
		}
		resp.NextCursor = t + "," + strconv.FormatInt(last.ID, 10)
	}

	if cacheKey != "" && h.Redis != nil {
		data, _ := json.Marshal(resp)
		h.Redis.Set(context.Background(), cacheKey, data, ttlFirstPage)
	}

	c.JSON(http.StatusOK, resp)
}

// ── shared helpers ──────────────────────────────────────

func toPhotoItem(p *model.Photo) PhotoItem {
	return PhotoItem{
		ID:           p.ID,
		ThumbnailURL: "/api/thumbnails/" + strconv.FormatInt(p.ID, 10) + ".webp",
		FileName:     filepath.Base(p.FilePath),
		FilePath:     p.FilePath,
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
