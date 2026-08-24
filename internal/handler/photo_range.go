package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"shutterseek/internal/service"
)

// PhotoRange returns the ids of all photos between two anchor photos in the
// current list order (taken_at DESC NULLS LAST, id DESC), inclusive of both
// ends. Order-independent: from/to may be swapped.
// GET /api/v1/photos/range?from_id=&to_id=[&album_id=&month=]
func (h *Handler) PhotoRange(c *gin.Context) {
	fromID, err1 := strconv.ParseInt(c.Query("from_id"), 10, 64)
	toID, err2 := strconv.ParseInt(c.Query("to_id"), 10, 64)
	if err1 != nil || err2 != nil || fromID <= 0 || toID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from_id/to_id"})
		return
	}
	ids, err := h.PhotoSvc.PhotoRange(c.Request.Context(), service.RangeParams{
		FromID:  fromID,
		ToID:    toID,
		AlbumID: c.Query("album_id"),
		Month:   c.Query("month"),
		Role:    c.GetString("role"),
	})
	switch {
	case errors.Is(err, service.ErrAnchorNotFound):
		c.JSON(http.StatusBadRequest, gin.H{"error": "anchor photos not found"})
		return
	case err != nil:
		var tooLarge service.RangeTooLargeError
		if errors.As(err, &tooLarge) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "range too large", "count": tooLarge.Count})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"photo_ids": ids, "count": len(ids)})
}
