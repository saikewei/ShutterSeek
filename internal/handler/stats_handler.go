package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// AdminStats returns the read-only system snapshot for the admin console.
// GET /api/v1/admin/stats
//
// The service never fails, so a dependency being down shows up as a false
// health flag rather than an error response.
func (h *Handler) AdminStats(c *gin.Context) {
	c.JSON(http.StatusOK, h.StatsSvc.Snapshot(c.Request.Context()))
}
