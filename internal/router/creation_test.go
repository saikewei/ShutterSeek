package router

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"shutterseek/internal/handler"
	"shutterseek/internal/middleware"
	"shutterseek/internal/service"
	"testing"
	"time"
)

func TestCreationRoutesRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := Setup(&handler.Handler{}, t.TempDir(), t.TempDir())
	for _, path := range []string{"/api/v1/creations", "/api/v1/creations/capabilities", "/api/v1/creations/work", "/api/v1/creations/work/preview", "/api/v1/creations/work/screenshots/asset", "/api/v1/creations/work/runs/run/events", "/api/v1/creations/work/versions", "/api/v1/creations/work/messages", "/api/v1/creations/shared/share", "/api/v1/creations/shared/share/preview", "/api/v1/creations/work/share"} {
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
func TestAuthenticatedGuestCanReachCreationRoutesWithoutAdminPrivileges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: raw}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	auth := service.NewAuthService(db, "test-only-signing-key")
	token, err := auth.GenerateToken(7, "guest")
	if err != nil {
		t.Fatal(err)
	}
	engine := Setup(&handler.Handler{AuthSvc: auth}, t.TempDir(), t.TempDir())
	for _, test := range []struct {
		path   string
		status int
	}{
		{"/api/v1/creations/capabilities", http.StatusOK},
		{"/api/v1/creations", http.StatusServiceUnavailable},
		{"/api/v1/creations/shared/share", http.StatusServiceUnavailable},
		{"/api/v1/creations/shared/share/preview", http.StatusServiceUnavailable},
		{"/api/v1/admin/stats", http.StatusForbidden},
	} {
		mock.ExpectQuery(`SELECT "updated_at" FROM "users" WHERE "users"."id" = \$1`).
			WithArgs(int64(7), 1).WillReturnRows(sqlmock.NewRows([]string{"updated_at"}).AddRow(time.Now().Add(-time.Hour)))
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		request.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Fatalf("%s: status=%d", test.path, response.Code)
		}
		if response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
			t.Fatalf("%s fell through to SPA", test.path)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
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
	if response.Body.String() != `{"enabled":false,"max_photos":200,"sharing_enabled":false}` {
		t.Fatal(response.Body.String())
	}
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/creations", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatal(response.Code)
	}
}
