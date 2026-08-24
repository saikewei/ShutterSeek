package handler

import (
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"shutterseek/internal/model"
	"shutterseek/internal/service"
)

// AlbumDates returns date distribution for photos within an album.
// GET /api/v1/albums/:id/dates
func (h *Handler) AlbumDates(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	rows, err := h.AlbumSvc.AlbumDates(c.Request.Context(), c.GetString("role"), id)
	if errors.Is(err, service.ErrAlbumNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "album not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}
	c.JSON(http.StatusOK, rows)
}

// ── Album List ──────────────────────────────────────────

type AlbumItem struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	CoverURL    string    `json:"cover_url"`
	PhotoCount  int64     `json:"photo_count"`
	SortOrder   int32     `json:"sort_order"`
	IsPublic    bool      `json:"is_public"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ListAlbums returns all albums (guests see only public ones).
// GET /api/v1/albums
func (h *Handler) ListAlbums(c *gin.Context) {
	items, err := h.AlbumSvc.ListAlbums(c.Request.Context(), c.GetString("role"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list albums failed"})
		return
	}
	resp := make([]AlbumItem, len(items))
	for i, it := range items {
		resp[i] = AlbumItem(it)
	}
	c.JSON(http.StatusOK, gin.H{
		"items": resp,
		"total": len(resp),
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
	if c.GetString("role") == "guest" && !item.IsPublic {
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
	HeadCount  int         `json:"head_count,omitempty"`
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
	page, err := h.AlbumSvc.ListAlbumPhotosPage(
		c.Request.Context(), c.GetString("role"), albumID, limit,
		c.Query("cursor"), c.Query("month"),
	)
	if errors.Is(err, service.ErrAlbumNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "album not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}
	all := make([]PhotoItem, 0, len(page.HeadPhotos)+len(page.Photos))
	for i := len(page.HeadPhotos) - 1; i >= 0; i-- {
		all = append(all, toPhotoItem(&page.HeadPhotos[i]))
	}
	for _, p := range page.Photos {
		all = append(all, toPhotoItem(&p))
	}

	resp := AlbumPhotoListResponse{
		Items:      all,
		Total:      page.Total,
		NextCursor: "",
		HeadCount:  len(page.HeadPhotos),
	}
	if page.HasMore && len(page.Photos) > 0 {
		last := page.Photos[len(page.Photos)-1]
		resp.NextCursor = service.BuildNextCursor(last.TakenAt, last.ID)
	}
	c.JSON(http.StatusOK, resp)
}

// ── shared helpers ──────────────────────────────────────

// ── Album Mutations ────────────────────────────────────

type createAlbumReq struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	IsPublic    bool   `json:"is_public"`
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

	// Create always starts private; apply is_public immediately if requested
	if req.IsPublic {
		public := true
		item, err = h.AlbumSvc.UpdateAlbum(item.ID, nil, nil, nil, &public)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "create failed"})
			return
		}
	}
	// Album list changed
	c.JSON(http.StatusCreated, AlbumItem(*item))
}

type updateAlbumReq struct {
	Title        *string `json:"title"`
	Description  *string `json:"description"`
	CoverPhotoID *int64  `json:"cover_photo_id"`
	IsPublic     *bool   `json:"is_public"`
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

	item, err := h.AlbumSvc.UpdateAlbum(id, req.Title, req.Description, req.CoverPhotoID, req.IsPublic)
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
	// is_public changes visibility → clear all scoped photo caches
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
	// Album list changed
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// BatchRemovePhotos removes multiple photos from an album.
// DELETE /api/v1/albums/:id/photos
func (h *Handler) BatchRemovePhotos(c *gin.Context) {
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

	removed, err := h.AlbumSvc.BatchRemovePhotos(albumID, req.PhotoIDs)
	if err != nil {
		if errors.Is(err, service.ErrAlbumNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "album not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "remove failed"})
		return
	}

	// Album membership changed → clear scoped caches
	c.JSON(http.StatusOK, gin.H{"removed": removed})
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
	c.JSON(http.StatusOK, gin.H{"ok": true})
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
