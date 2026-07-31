package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"shutterseek/internal/model"
)

// rangeSelectLimit caps how many photos a single range gesture may select.
const rangeSelectLimit = 5000

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

	// Resolve the anchors' (taken_at, id) tuples.
	var anchors []model.Photo
	if err := h.DB.Select("id, taken_at").Where("id IN ?", []int64{fromID, toID}).Find(&anchors).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}
	if len(anchors) != 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "anchor photos not found"})
		return
	}

	// Order the anchors by list order. loT/loID = later in the DESC view
	// (smaller tuple), hiT/hiID = earlier (larger tuple).
	var a, b model.Photo
	if anchors[0].ID == fromID {
		a, b = anchors[0], anchors[1]
	} else {
		a, b = anchors[1], anchors[0]
	}
	loT, loID, hiT, hiID := a.TakenAt, a.ID, b.TakenAt, b.ID
	if photoTupleLess(b.TakenAt, b.ID, a.TakenAt, a.ID) {
		loT, loID, hiT, hiID = b.TakenAt, b.ID, a.TakenAt, a.ID
	}

	q := `SELECT id FROM photos
	      WHERE (COALESCE(taken_at, 'epoch'), id) >= (?, ?)
	        AND (COALESCE(taken_at, 'epoch'), id) <= (?, ?)`
	args := []interface{}{loT, loID, hiT, hiID}

	// Forward the current view's filter context
	albumIDStr := c.Query("album_id")
	uncategorized := albumIDStr == "none"
	if albumIDStr != "" && !uncategorized {
		if albumID, err := strconv.ParseInt(albumIDStr, 10, 64); err == nil && albumID > 0 {
			q += " AND id IN (SELECT photo_id FROM album_photos WHERE album_id = ?)"
			args = append(args, albumID)
		}
	}
	if uncategorized {
		q += " AND id NOT IN (SELECT DISTINCT photo_id FROM album_photos)"
	}
	if month := c.Query("month"); month != "" {
		if t, err := time.Parse("2006-01", month); err == nil {
			q += " AND taken_at < ?"
			args = append(args, t.AddDate(0, 1, 0))
		}
	}
	// Defense-in-depth for guests (route is admin-only)
	if c.GetString("role") == "guest" {
		q += " AND id IN (SELECT ap.photo_id FROM album_photos ap JOIN albums a ON a.id = ap.album_id WHERE a.is_public = true)"
	}
	q += " ORDER BY taken_at DESC NULLS LAST, id DESC"

	var ids []int64
	if err := h.DB.Raw(q, args...).Scan(&ids).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}
	if len(ids) > rangeSelectLimit {
		c.JSON(http.StatusBadRequest, gin.H{"error": "range too large", "count": len(ids)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"photo_ids": ids, "count": len(ids)})
}

// photoTupleLess reports whether (t1, id1) sorts before (t2, id2) in the
// taken_at ASC, id ASC sense. A zero taken_at (NULL) counts as oldest, which
// matches the COALESCE(taken_at, 'epoch') used in the SQL comparison.
func photoTupleLess(t1 time.Time, id1 int64, t2 time.Time, id2 int64) bool {
	if !t1.Equal(t2) {
		return t1.Before(t2)
	}
	return id1 < id2
}
