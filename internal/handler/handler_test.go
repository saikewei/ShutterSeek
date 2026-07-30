//go:build integration

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"shutterseek/internal/service"
)

func setupHandler(t *testing.T) *Handler {
	t.Helper()
	dsn := "postgres://photo_user:PhotoHyc65319436@postgres-main:5432/photo_search?sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	return &Handler{DB: db, AlbumSvc: service.NewAlbumService(db)}
}

func TestPhotoDates_ReturnsData(t *testing.T) {
	h := setupHandler(t)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/photos/dates", nil)

	h.PhotoDates(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var rows []DateCount
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected some dates")
	}
	// Verify descending order
	for i := 1; i < len(rows); i++ {
		if rows[i].Date > rows[i-1].Date {
			t.Fatalf("dates not descending: %s after %s", rows[i].Date, rows[i-1].Date)
		}
	}
	// Verify format YYYY-MM-DD
	for _, r := range rows {
		if len(r.Date) != 10 || r.Date[4] != '-' || r.Date[7] != '-' {
			t.Fatalf("bad date format: %s", r.Date)
		}
		if r.Count <= 0 {
			t.Fatalf("non-positive count for %s: %d", r.Date, r.Count)
		}
	}
	t.Logf("%d dates returned, total count: sum=%d", len(rows), sumCounts(rows))
}

func TestPhotoDates_ContentType(t *testing.T) {
	h := setupHandler(t)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/photos/dates", nil)

	h.PhotoDates(c)

	if ct := w.Header().Get("Content-Type"); ct == "" {
		t.Fatal("no Content-Type header")
	}
}

func TestPhotoDates_SumMatchesPhotos(t *testing.T) {
	h := setupHandler(t)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/photos/dates", nil)

	h.PhotoDates(c)

	var rows []DateCount
	json.Unmarshal(w.Body.Bytes(), &rows)

	sum := sumCounts(rows)

	// Total photos with taken_at
	var dbTotal int64
	h.DB.Raw("SELECT COUNT(*) FROM photos WHERE taken_at IS NOT NULL").Scan(&dbTotal)

	if sum != dbTotal {
		t.Fatalf("sum of date counts (%d) != photos with taken_at (%d)", sum, dbTotal)
	}
}

func sumCounts(rows []DateCount) int64 {
	var s int64
	for _, r := range rows {
		s += r.Count
	}
	return s
}
