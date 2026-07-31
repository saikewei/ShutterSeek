package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"shutterseek/internal/middleware"
	"shutterseek/internal/model"
	"shutterseek/internal/service"
)

// ── Auth ────────────────────────────────────────────────

type loginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login authenticates a user and sets a JWT cookie.
// POST /api/v1/auth/login
func (h *Handler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求"})
		return
	}

	u, err := h.AuthSvc.FindUserByUsername(req.Username)
	if err != nil || !h.AuthSvc.CheckPassword(u.PasswordHash, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}

	token, err := h.AuthSvc.GenerateToken(u.ID, u.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成token失败"})
		return
	}

	middleware.SetTokenCookie(c, token)
	h.AuthSvc.LogEvent(u.ID, u.Username, model.LogEventLogin, c.ClientIP())
	c.JSON(http.StatusOK, gin.H{
		"id":       u.ID,
		"username": u.Username,
		"role":     u.Role,
	})
}

// Logout clears the JWT cookie.
// POST /api/v1/auth/logout
func (h *Handler) Logout(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if u, err := h.AuthSvc.FindUserByID(userID); err == nil {
		h.AuthSvc.LogEvent(u.ID, u.Username, model.LogEventLogout, c.ClientIP())
	}
	middleware.ClearTokenCookie(c)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Me returns the current user's info.
// GET /api/v1/auth/me
func (h *Handler) Me(c *gin.Context) {
	userID := c.GetInt64("user_id")
	u, err := h.AuthSvc.FindUserByID(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}
	// JWT session restore counts as an activity event
	h.AuthSvc.LogEvent(u.ID, u.Username, model.LogEventSession, c.ClientIP())
	c.JSON(http.StatusOK, gin.H{
		"id":       u.ID,
		"username": u.Username,
		"role":     u.Role,
	})
}

// ── Invites ─────────────────────────────────────────────

// CreateInvite generates a new invite code.
// POST /api/v1/invites  [Admin]
func (h *Handler) CreateInvite(c *gin.Context) {
	detail, err := h.AuthSvc.CreateInviteCode(c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建邀请码失败"})
		return
	}
	c.JSON(http.StatusCreated, detail)
}

// ListInvites returns all invite codes.
// GET /api/v1/invites  [Admin]
func (h *Handler) ListInvites(c *gin.Context) {
	list, err := h.AuthSvc.ListInviteCodes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	if list == nil {
		list = []service.InviteDetail{}
	}
	c.JSON(http.StatusOK, gin.H{"items": list})
}

// DeleteInvite revokes an invite code.
// DELETE /api/v1/invites/:id  [Admin]
func (h *Handler) DeleteInvite(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的id"})
		return
	}
	if err := h.AuthSvc.DeleteInviteCode(id); err != nil {
		if errors.Is(err, service.ErrInviteInvalid) || errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "邀请码不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ValidateInvite checks whether an invite code is valid and unexpired.
// GET /api/v1/invites/validate/:code  (no auth)
func (h *Handler) ValidateInvite(c *gin.Context) {
	code := c.Param("code")
	err := h.AuthSvc.ValidateInviteCode(code)
	valid := err == nil
	status := "valid"
	if errors.Is(err, service.ErrInviteExpired) {
		status = "expired"
	} else if err != nil {
		status = "invalid"
	}
	c.JSON(http.StatusOK, gin.H{"valid": valid, "status": status})
}

// ListLogs returns user activity logs, newest first.
// GET /api/v1/auth/logs  [Admin]
func (h *Handler) ListLogs(c *gin.Context) {
	limit := 100
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	var beforeID int64
	if b := c.Query("before_id"); b != "" {
		beforeID, _ = strconv.ParseInt(b, 10, 64)
	}

	logs, total, err := h.AuthSvc.ListLogs(limit, beforeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	if logs == nil {
		logs = []model.UserLog{}
	}
	c.JSON(http.StatusOK, gin.H{"items": logs, "total": total})
}

type redeemReq struct {
	Code     string `json:"code" binding:"required"`
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// RedeemInvite creates a guest account from an invite code.
// POST /api/v1/invites/redeem  (no auth required)
func (h *Handler) RedeemInvite(c *gin.Context) {
	var req redeemReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求"})
		return
	}

	u, err := h.AuthSvc.RedeemInviteCode(req.Code, req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	token, err := h.AuthSvc.GenerateToken(u.ID, u.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成token失败"})
		return
	}

	middleware.SetTokenCookie(c, token)
	c.JSON(http.StatusCreated, gin.H{
		"id":       u.ID,
		"username": u.Username,
		"role":     u.Role,
	})
}
