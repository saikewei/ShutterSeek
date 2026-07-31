package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"shutterseek/internal/service"
)

// CookieName is the HttpOnly cookie that carries the JWT.
const CookieName = "shutterseek_token"

// AuthRequired validates the JWT cookie and injects user info into the context.
func AuthRequired(authSvc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr, err := c.Cookie(CookieName)
		if err != nil || tokenStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		claims, err := authSvc.ValidateToken(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "登录已过期"})
			return
		}
		c.Set("user_id", claims.UserID)
		c.Set("role", claims.Role)
		c.Next()
	}
}

// AdminOnly ensures the authenticated user has the admin role.
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("role")
		if role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "需要管理员权限"})
			return
		}
		c.Next()
	}
}

// SetTokenCookie writes the JWT as an HttpOnly cookie.
func SetTokenCookie(c *gin.Context, token string) {
	c.SetCookie(
		CookieName,
		token,
		30*24*3600, // 30 days
		"/api",
		"",
		false, // secure — false for local dev; production sits behind Tailscale
		true,  // httpOnly
	)
}

// ClearTokenCookie removes the JWT cookie.
func ClearTokenCookie(c *gin.Context) {
	c.SetCookie(CookieName, "", -1, "/api", "", false, true)
}
