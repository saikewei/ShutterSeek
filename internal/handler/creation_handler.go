package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"shutterseek/internal/service"
)

func creationUser(c *gin.Context) int64 { v, _ := c.Get("user_id"); id, _ := v.(int64); return id }
func creationError(c *gin.Context, err error) {
	code := http.StatusInternalServerError
	message := "创作服务暂时不可用"
	if errors.Is(err, gorm.ErrRecordNotFound) {
		code = http.StatusNotFound
		message = "作品或资源不存在"
	}
	if errors.Is(err, service.ErrCreationInvalid) {
		code = http.StatusBadRequest
		message = "请求无效，请选择 1–200 张照片"
	}
	if errors.Is(err, service.ErrCreationConflict) {
		code = http.StatusConflict
		message = "已有任务正在执行，或作品内容已更新，请刷新"
	}
	c.JSON(code, gin.H{"error": message})
}
func (h *Handler) CreationCapabilities(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"enabled": h.CreationSvc != nil, "max_photos": 200})
}
func (h *Handler) CreationEnabled(c *gin.Context) {
	if h.CreationSvc == nil {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "AI 创作尚未启用"})
		return
	}
	c.Next()
}
func (h *Handler) ListCreations(c *gin.Context) {
	out, err := h.CreationSvc.List(c.Request.Context(), creationUser(c))
	if err != nil {
		creationError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
func (h *Handler) CreateCreation(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var req service.CreationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		creationError(c, service.ErrCreationInvalid)
		return
	}
	out, err := h.CreationSvc.Create(c.Request.Context(), creationUser(c), req)
	if err != nil {
		creationError(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}
func (h *Handler) GetCreation(c *gin.Context) {
	out, err := h.CreationSvc.Detail(c.Request.Context(), creationUser(c), c.Param("creation"))
	if err != nil {
		creationError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
func (h *Handler) RenameCreation(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<10)
	var req service.CreationTitleRequest
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入 1–100 个字符的作品名称"})
		return
	}
	err := h.CreationSvc.Rename(c.Request.Context(), creationUser(c), c.Param("creation"), req)
	if errors.Is(err, service.ErrCreationInvalid) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入 1–100 个字符的作品名称"})
		return
	}
	if err != nil {
		creationError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (h *Handler) UpdateCreationPhotos(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 24<<10)
	var req service.CreationPhotosRequest
	if c.ShouldBindJSON(&req) != nil {
		creationError(c, service.ErrCreationInvalid)
		return
	}
	if err := h.CreationSvc.UpdatePhotos(c.Request.Context(), creationUser(c), c.Param("creation"), req); err != nil {
		creationError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (h *Handler) ResetCreation(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<10)
	var req service.CreationResetRequest
	if c.ShouldBindJSON(&req) != nil {
		creationError(c, service.ErrCreationInvalid)
		return
	}
	if err := h.CreationSvc.Reset(c.Request.Context(), creationUser(c), c.Param("creation"), req); err != nil {
		creationError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (h *Handler) CreationPhotoContent(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("photo"), 10, 64)
	if err != nil || id <= 0 {
		creationError(c, service.ErrCreationInvalid)
		return
	}
	b, mime, err := h.CreationSvc.PhotoContent(c.Request.Context(), creationUser(c), c.Param("creation"), id)
	if err != nil {
		creationError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Data(http.StatusOK, mime, b)
}
func (h *Handler) StartCreationRun(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var req service.CreationRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		creationError(c, service.ErrCreationInvalid)
		return
	}
	out, err := h.CreationSvc.Run(c.Request.Context(), creationUser(c), c.Param("creation"), req)
	if err != nil {
		creationError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, out)
}
func (h *Handler) GetCreationRun(c *gin.Context) {
	out, err := h.CreationSvc.RunOwned(c.Request.Context(), creationUser(c), c.Param("creation"), c.Param("run"))
	if err != nil {
		creationError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
func (h *Handler) CancelCreationRun(c *gin.Context) {
	err := h.CreationSvc.Cancel(c.Request.Context(), creationUser(c), c.Param("creation"), c.Param("run"))
	if err != nil {
		creationError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (h *Handler) CreationEvents(c *gin.Context) {
	cursor := c.GetHeader("Last-Event-ID")
	if cursor == "" {
		cursor = c.Query("after")
	}
	after, _ := strconv.ParseInt(cursor, 10, 64)
	user := creationUser(c)
	work := c.Param("creation")
	run := c.Param("run")
	if _, err := h.CreationSvc.RunOwned(c.Request.Context(), user, work, run); err != nil {
		creationError(c, err)
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-store")
	c.Header("X-Accel-Buffering", "no")
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		events, state, err := h.CreationSvc.Events(c.Request.Context(), user, work, run, after)
		if err != nil {
			return
		}
		for _, e := range events {
			fmt.Fprintf(c.Writer, "id: %d\nevent: progress\ndata: %s\n\n", e.ID, e.Data)
			after = e.ID
		}
		if state != "queued" && state != "running" && len(events) < 100 {
			b, _ := json.Marshal(gin.H{"status": state})
			fmt.Fprintf(c.Writer, "event: done\ndata: %s\n\n", b)
			c.Writer.Flush()
			return
		}
		fmt.Fprint(c.Writer, ": heartbeat\n\n")
		c.Writer.Flush()
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
func (h *Handler) RestoreCreationVersion(c *gin.Context) {
	var req struct {
		BaseVersion string `json:"base_version"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		creationError(c, service.ErrCreationInvalid)
		return
	}
	err := h.CreationSvc.Restore(c.Request.Context(), creationUser(c), c.Param("creation"), c.Param("version"), req.BaseVersion)
	if err != nil {
		creationError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (h *Handler) CreationPreview(c *gin.Context) {
	html, err := h.CreationSvc.Preview(c.Request.Context(), creationUser(c), c.Param("creation"), c.Query("version"))
	if err != nil {
		creationError(c, err)
		return
	}
	c.Header("Content-Security-Policy", service.CreationCSP)
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=(), clipboard-read=(), clipboard-write=()")
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}
func (h *Handler) CreationScreenshot(c *gin.Context) {
	data, err := h.CreationSvc.Screenshot(c.Request.Context(), creationUser(c), c.Param("creation"), c.Param("asset"))
	if err != nil {
		creationError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "image/png", data)
}

func (h *Handler) CreationMessages(c *gin.Context) {
	out, err := h.CreationSvc.Detail(c.Request.Context(), creationUser(c), c.Param("creation"))
	if err != nil {
		creationError(c, err)
		return
	}
	c.JSON(http.StatusOK, out["messages"])
}
func (h *Handler) CreationVersions(c *gin.Context) {
	out, err := h.CreationSvc.Detail(c.Request.Context(), creationUser(c), c.Param("creation"))
	if err != nil {
		creationError(c, err)
		return
	}
	c.JSON(http.StatusOK, out["versions"])
}
