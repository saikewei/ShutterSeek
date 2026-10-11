package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"shutterseek/internal/service"
)

func (h *Handler) GetCreationShare(c *gin.Context) {
	share, err := h.CreationSvc.OwnerShare(c.Request.Context(), creationUser(c), c.Param("creation"))
	if err != nil {
		creationError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, share)
}
func (h *Handler) EnableCreationShare(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<10)
	var req service.CreationShareRequest
	if c.ShouldBindJSON(&req) != nil {
		creationError(c, service.ErrCreationInvalid)
		return
	}
	share, err := h.CreationSvc.EnableShare(c.Request.Context(), creationUser(c), c.Param("creation"), req)
	if err != nil {
		creationError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, share)
}
func (h *Handler) RevokeCreationShare(c *gin.Context) {
	if err := h.CreationSvc.RevokeShare(c.Request.Context(), creationUser(c), c.Param("creation")); err != nil {
		creationError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (h *Handler) GetSharedCreation(c *gin.Context) {
	out, err := h.CreationSvc.Shared(c.Request.Context(), creationUser(c), c.Param("share"))
	if err != nil {
		creationError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, out)
}
func (h *Handler) SharedCreationPreview(c *gin.Context) {
	html, err := h.CreationSvc.SharedPreview(c.Request.Context(), creationUser(c), c.Param("share"))
	if err != nil {
		creationError(c, err)
		return
	}
	creationPreviewResponse(c, html)
}
