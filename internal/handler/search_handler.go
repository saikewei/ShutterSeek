package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"shutterseek/internal/service"
)

type SearchPhotoItem struct {
	PhotoItem
	Score float64 `json:"score"`
}

// Search performs semantic or time-scoped photo search.
// GET /api/v1/search?q=sea&limit=100&album_id=9&from=2020-06-01&to=2020-08-31&diversify=true
func (h *Handler) Search(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	fromStr := strings.TrimSpace(c.Query("from"))
	toStr := strings.TrimSpace(c.Query("to"))
	fromTime, toTime, err := service.ParseSearchDateRange(fromStr, toStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的时间范围"})
		return
	}
	if q == "" && fromTime == nil && toTime == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing query"})
		return
	}

	limit := 100
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 200 {
				n = 200
			}
			limit = n
		}
	}
	albumID, _ := strconv.ParseInt(c.Query("album_id"), 10, 64)
	role := c.GetString("role")
	diversify := c.Query("diversify") == "true" || c.Query("diversify") == "1"

	if role == "guest" && albumID > 0 {
		exists, public, err := h.AlbumSvc.GetAlbumVisibility(albumID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
			return
		}
		if !exists || !public {
			c.JSON(http.StatusForbidden, gin.H{"error": "无权访问"})
			return
		}
	}

	req := service.SearchQuery{
		Query:     q,
		Role:      role,
		From:      fromTime,
		To:        toTime,
		Limit:     limit,
		Diversify: diversify,
	}
	if albumID > 0 {
		req.AlbumIDs = []int64{albumID}
	}

	res, err := h.SearchSvc.Query(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrEmptyQuery):
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing query"})
		case errors.Is(err, service.ErrInvalidDateRange):
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的时间范围"})
		case errors.Is(err, service.ErrEmbedUnavailable):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "搜索服务不可用"})
		case errors.Is(err, service.ErrEmbedInvalid):
			c.JSON(http.StatusBadGateway, gin.H{"error": "搜索服务响应异常"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "搜索失败"})
		}
		return
	}

	out := make([]SearchPhotoItem, 0, len(res.Items))
	for _, it := range res.Items {
		out = append(out, SearchPhotoItem{PhotoItem: toPhotoItem(&it.Photo), Score: it.Score})
	}
	c.JSON(http.StatusOK, gin.H{"items": out, "total": len(out), "meta": res.Meta})
}
