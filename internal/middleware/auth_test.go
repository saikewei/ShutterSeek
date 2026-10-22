package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTokenCookieSecureFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	old := CookieSecure
	t.Cleanup(func() { CookieSecure = old })

	for _, secure := range []bool{false, true} {
		CookieSecure = secure
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		SetTokenCookie(c, "test-token")

		cookies := w.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatalf("secure=%t: got %d cookies, want 1", secure, len(cookies))
		}
		cookie := cookies[0]
		if cookie.Secure != secure {
			t.Errorf("secure=%t: cookie Secure=%t", secure, cookie.Secure)
		}
		if !cookie.HttpOnly {
			t.Errorf("secure=%t: cookie is not HttpOnly", secure)
		}
		if cookie.Path != "/api" {
			t.Errorf("secure=%t: cookie path=%q, want /api", secure, cookie.Path)
		}
	}
}
