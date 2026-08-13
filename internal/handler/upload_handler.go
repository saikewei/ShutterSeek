package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"shutterseek/internal/service"
)

const maxUploadBytes = 200 << 20 // 200MB

// Upload 处理 multipart 上传：file + vector。
// POST /api/v1/photos/upload（admin only）
func (h *Handler) Upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing file"})
		return
	}
	vecStr := c.PostForm("vector")
	if vecStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing vector"})
		return
	}
	f, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "open file"})
		return
	}
	defer f.Close()

	p, err := h.UploadSvc.Upload(c.Request.Context(), f, fileHeader.Filename, vecStr)
	switch {
	case errors.Is(err, service.ErrInvalidVector):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "invalid vector"})
		return
	case err != nil:
		var dup service.ErrDuplicate
		if errors.As(err, &dup) {
			c.JSON(http.StatusConflict, gin.H{
				"duplicate":   true,
				"existing_id": dup.ExistingID,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":            p.ID,
		"file_path":     p.FilePath,
		"taken_at":      p.TakenAt.Format("2006-01-02T15:04:05Z07:00"),
		"width":         p.Width,
		"height":        p.Height,
		"thumbnail_url": fmt.Sprintf("/api/thumbnails/%d.webp", p.ID),
		"duplicate":     false,
	})
}
