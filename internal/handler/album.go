package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"shutterseek/internal/model"
	"shutterseek/internal/service"
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

// ── Album Mutations ────────────────────────────────────

type createAlbumReq struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
}

// CreateAlbum creates a new album.
// POST /api/v1/albums
func (h *Handler) CreateAlbum(c *gin.Context) {
	var req createAlbumReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}

	item, err := h.AlbumSvc.CreateAlbum(req.Title, req.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create failed"})
		return
	}
	c.JSON(http.StatusCreated, AlbumItem(*item))
}

type updateAlbumReq struct {
	Title        *string `json:"title"`
	Description  *string `json:"description"`
	CoverPhotoID *int64  `json:"cover_photo_id"`
}

// UpdateAlbum updates an album.
// PUT /api/v1/albums/:id
func (h *Handler) UpdateAlbum(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req updateAlbumReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}

	item, err := h.AlbumSvc.UpdateAlbum(id, req.Title, req.Description, req.CoverPhotoID)
	if err != nil {
		if errors.Is(err, service.ErrAlbumNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "album not found"})
			return
		}
		if errors.Is(err, service.ErrPhotoNotInAlbum) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cover photo not in this album"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update failed"})
		return
	}
	c.JSON(http.StatusOK, AlbumItem(*item))
}

// DeleteAlbum deletes an album.
// DELETE /api/v1/albums/:id
func (h *Handler) DeleteAlbum(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.AlbumSvc.DeleteAlbum(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// RemoveAlbumPhoto removes a photo from an album.
// DELETE /api/v1/albums/:id/photos/:photo_id
func (h *Handler) RemoveAlbumPhoto(c *gin.Context) {
	albumID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid album id"})
		return
	}
	photoID, err := strconv.ParseInt(c.Param("photo_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid photo id"})
		return
	}

	if err := h.AlbumSvc.RemoveAlbumPhoto(albumID, photoID); err != nil {
		if errors.Is(err, service.ErrPhotoNotInAlbum) {
			c.JSON(http.StatusNotFound, gin.H{"error": "photo not in album"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "remove failed"})
		return
	}

	// Invalidate album-photos Redis cache
	h.clearAlbumCache(albumID)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// clearAlbumCache removes all cached first-page responses for an album.
func (h *Handler) clearAlbumCache(albumID int64) {
	if h.Redis == nil {
		return
	}
	prefix := "cache:album_photos:" + strconv.FormatInt(albumID, 10) + ":"
	ctx := context.Background()

	var cursor uint64
	for {
		keys, next, err := h.Redis.Scan(ctx, cursor, prefix+"*", 100).Result()
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

// ── Batch Add Photos ────────────────────────────────────

type batchAddReq struct {
	PhotoIDs []int64 `json:"photo_ids" binding:"required"`
}

// BatchAddPhotos adds multiple photos to an album.
// POST /api/v1/albums/:id/photos
func (h *Handler) BatchAddPhotos(c *gin.Context) {
	albumID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid album id"})
		return
	}

	var req batchAddReq
	if err := c.ShouldBindJSON(&req); err != nil || len(req.PhotoIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "photo_ids required"})
		return
	}

	result, err := h.AlbumSvc.BatchAddPhotos(albumID, req.PhotoIDs)
	if err != nil {
		if errors.Is(err, service.ErrAlbumNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "album not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "batch add failed"})
		return
	}

	h.clearAlbumCache(albumID)
	c.JSON(http.StatusOK, gin.H{
		"added":   result.Added,
		"skipped": result.Skipped,
	})
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
