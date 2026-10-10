package router

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"shutterseek/internal/handler"
	"shutterseek/internal/middleware"
	"testing"
)

func TestCreationRoutesRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := Setup(&handler.Handler{}, t.TempDir(), t.TempDir())
	for _, path := range []string{"/api/v1/creations", "/api/v1/creations/capabilities", "/api/v1/creations/work", "/api/v1/creations/work/preview", "/api/v1/creations/work/screenshots/asset", "/api/v1/creations/work/runs/run/events", "/api/v1/creations/work/versions", "/api/v1/creations/work/messages"} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s: status=%d", path, response.Code)
		}
		if response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
			t.Fatalf("%s fell through to SPA", path)
		}
	}
}
func TestGuestCannotReachCreationService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	h := &handler.Handler{}
	engine.Use(func(c *gin.Context) { c.Set("role", "guest") })
	engine.Use(middleware.AdminOnly())
	engine.GET("/creations", h.CreationEnabled, h.ListCreations)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/creations", nil))
	if response.Code != http.StatusForbidden {
		t.Fatalf("guest: status=%d", response.Code)
	}
}
func TestCreationFeatureDefaultsOff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	h := &handler.Handler{}
	engine.GET("/capabilities", h.CreationCapabilities)
	engine.GET("/creations", h.CreationEnabled, h.ListCreations)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/capabilities", nil))
	if response.Body.String() != `{"enabled":false,"max_photos":200}` {
		t.Fatal(response.Body.String())
	}
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/creations", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatal(response.Code)
	}
}
