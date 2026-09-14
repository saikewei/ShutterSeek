package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// Thumbnail serves one photo's webp thumbnail.
//
//	GET /api/v1/thumbnails/:file   (file is "<id>.webp")
//
// This route used to be registered with r.Static on the root engine, i.e.
// entirely outside the auth middleware, and it had a second public alias at
// /thumbnails. Anyone who could reach the port could walk the ids and download
// the whole library's thumbnails without a session. It now lives inside the
// authenticated group and applies the same guest rule as the original endpoint.
func (h *Handler) Thumbnail(c *gin.Context) {
	id, ok := parseThumbFile(c.Param("file"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid thumbnail"})
		return
	}

	if c.GetString("role") == "guest" {
		// Fail closed: without the visibility check there is no way to tell an
		// allowed thumbnail from a private one, so serve nothing.
		if h.PhotoSvc == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
			return
		}
		allowed, err := h.PhotoSvc.PhotoInPublicAlbum(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
			return
		}
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": "无权访问"})
			return
		}
	}

	path := filepath.Join(h.ThumbnailsDir, fmt.Sprintf("%d.webp", id))
	if _, err := os.Stat(path); err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	// Authenticated content: keep it out of shared caches. http.ServeFile still
	// honours If-Modified-Since, so revisits stay free.
	c.Header("Cache-Control", "private, max-age=86400")
	c.File(path)
}

// parseThumbFile accepts exactly "<positive int>.webp" and nothing else, so no
// other path can be reached through this route.
func parseThumbFile(name string) (int64, bool) {
	base, found := strings.CutSuffix(name, ".webp")
	if !found || base == "" || len(base) > 19 {
		return 0, false
	}
	for i := 0; i < len(base); i++ {
		if base[i] < '0' || base[i] > '9' {
			return 0, false
		}
	}
	id, err := strconv.ParseInt(base, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}
