//go:build integration

package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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
	return &Handler{
		DB:       db,
		AlbumSvc: service.NewAlbumService(db, nil),
		PhotoSvc: service.NewPhotoService(db, nil, service.NewAlbumService(db, nil)),
	}
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

	var rows []service.DateCount
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

	var rows []service.DateCount
	json.Unmarshal(w.Body.Bytes(), &rows)

	sum := sumCounts(rows)

	// Total photos with taken_at
	var dbTotal int64
	h.DB.Raw("SELECT COUNT(*) FROM photos WHERE taken_at IS NOT NULL").Scan(&dbTotal)

	if sum != dbTotal {
		t.Fatalf("sum of date counts (%d) != photos with taken_at (%d)", sum, dbTotal)
	}
}

func sumCounts(rows []service.DateCount) int64 {
	var s int64
	for _, r := range rows {
		s += r.Count
	}
	return s
}

// ═══════════════════════════════════════════════════════
// ListPhotos filters
// ═══════════════════════════════════════════════════════

func ginCtx(method, url string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, url, nil)
	return c, w
}

func TestListPhotos_AlbumIDFilter(t *testing.T) {
	h := setupHandler(t)
	c, w := ginCtx("GET", "/api/v1/photos?album_id=1&limit=3")

	h.ListPhotos(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp PhotoListResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total == 0 {
		t.Fatal("expected non-zero total")
	}
	// Verify results differ from unfiltered
	c2, w2 := ginCtx("GET", "/api/v1/photos?album_id=10&limit=3")
	h.ListPhotos(c2)
	var resp2 PhotoListResponse
	json.Unmarshal(w2.Body.Bytes(), &resp2)
	if resp.Total == resp2.Total && len(resp.Items) > 0 && len(resp2.Items) > 0 &&
		resp.Items[0].ID == resp2.Items[0].ID {
		t.Fatal("different albums returned same results")
	}
	t.Logf("album 1: %d photos, album 10: %d photos", resp.Total, resp2.Total)
}

func TestListPhotos_Uncategorized(t *testing.T) {
	h := setupHandler(t)
	c, w := ginCtx("GET", "/api/v1/photos?album_id=none&limit=5")

	h.ListPhotos(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp PhotoListResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	// Uncategorized count should be <= total
	var total int64
	h.DB.Raw("SELECT COUNT(*) FROM photos WHERE taken_at IS NOT NULL").Scan(&total)
	if resp.Total > total {
		t.Fatalf("uncategorized count (%d) > total (%d)", resp.Total, total)
	}
	t.Logf("uncategorized: %d photos", resp.Total)
}

func TestListPhotos_WithAlbums(t *testing.T) {
	h := setupHandler(t)
	c, w := ginCtx("GET", "/api/v1/photos?with_albums=true&limit=5")

	h.ListPhotos(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp PhotoListResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	hasAlbumIDs := false
	for _, item := range resp.Items {
		if len(item.AlbumIDs) > 0 {
			hasAlbumIDs = true
			t.Logf("photo %d in albums: %v", item.ID, item.AlbumIDs)
			break
		}
	}
	if !hasAlbumIDs {
		t.Log("no album_ids found in first 5 photos (might all be uncategorized)")
	}
}

func TestListPhotos_MonthFilter(t *testing.T) {
	h := setupHandler(t)
	c, w := ginCtx("GET", "/api/v1/photos?month=2025-01&limit=5")

	h.ListPhotos(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp PhotoListResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total == 0 {
		t.Fatal("expected some photos in Jan 2025")
	}
	// Main photos (after head) should be before Feb 2025
	head := resp.HeadCount
	for i, item := range resp.Items {
		if i >= head && item.TakenAt > "2025-02-01" {
			t.Fatalf("main photo %d taken_at=%s is after month cutoff", item.ID, item.TakenAt)
		}
	}
	t.Logf("Jan 2025 and earlier: %d photos, head_count=%d", resp.Total, head)
}

func TestListPhotos_MonthWithAlbum(t *testing.T) {
	h := setupHandler(t)
	c, w := ginCtx("GET", "/api/v1/photos?album_id=1&month=2025-01&limit=5")

	h.ListPhotos(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp PhotoListResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	t.Logf("album 1, Jan 2025: %d photos", resp.Total)
}

func TestListPhotos_NewerThan(t *testing.T) {
	h := setupHandler(t)
	// Get a known photo's cursor
	c1, w1 := ginCtx("GET", "/api/v1/photos?limit=1")
	h.ListPhotos(c1)
	var r1 PhotoListResponse
	json.Unmarshal(w1.Body.Bytes(), &r1)
	if len(r1.Items) == 0 {
		t.Skip("no photos")
	}

	cursor := r1.Items[0].TakenAt + "," + itoa(r1.Items[0].ID)
	c2, w2 := ginCtx("GET", "/api/v1/photos?newer_than="+cursor+"&limit=5")
	h.ListPhotos(c2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}
	var r2 PhotoListResponse
	json.Unmarshal(w2.Body.Bytes(), &r2)
	t.Logf("newer_than returned %d photos", len(r2.Items))
}

func TestListPhotos_Pagination(t *testing.T) {
	h := setupHandler(t)

	// First page
	c1, w1 := ginCtx("GET", "/api/v1/photos?limit=3")
	h.ListPhotos(c1)
	var r1 PhotoListResponse
	json.Unmarshal(w1.Body.Bytes(), &r1)
	if len(r1.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(r1.Items))
	}
	if r1.NextCursor == "" {
		t.Fatal("expected next_cursor")
	}

	// Second page
	c2, w2 := ginCtx("GET", "/api/v1/photos?limit=3&cursor="+r1.NextCursor)
	h.ListPhotos(c2)
	var r2 PhotoListResponse
	json.Unmarshal(w2.Body.Bytes(), &r2)
	if len(r2.Items) == 0 {
		t.Fatal("expected items on second page")
	}
	// Verify no overlap
	for _, a := range r1.Items {
		for _, b := range r2.Items {
			if a.ID == b.ID {
				t.Fatalf("photo %d appears on both pages", a.ID)
			}
		}
	}
	t.Logf("page 1: %d items, page 2: %d items", len(r1.Items), len(r2.Items))
}

// ═══════════════════════════════════════════════════════
// PhotoDates with album filter
// ═══════════════════════════════════════════════════════

func TestPhotoDates_AlbumFilter(t *testing.T) {
	h := setupHandler(t)
	c1, w1 := ginCtx("GET", "/api/v1/photos/dates?album_id=1")
	h.PhotoDates(c1)

	c2, w2 := ginCtx("GET", "/api/v1/photos/dates?album_id=10")
	h.PhotoDates(c2)

	var rows1, rows2 []service.DateCount
	json.Unmarshal(w1.Body.Bytes(), &rows1)
	json.Unmarshal(w2.Body.Bytes(), &rows2)

	s1, s2 := sumCounts(rows1), sumCounts(rows2)
	if s1 == s2 && s1 > 0 {
		t.Fatal("different albums returned same date counts")
	}
	t.Logf("album 1: %d photos across %d dates, album 10: %d photos across %d dates",
		s1, len(rows1), s2, len(rows2))
}

// ═══════════════════════════════════════════════════════
// Album CRUD handlers
// ═══════════════════════════════════════════════════════

func TestCreateAlbumHandler(t *testing.T) {
	h := setupHandler(t)
	body := `{"title":"H_Create","description":"handler test"}`
	c, w := ginCtx("POST", "/api/v1/albums")
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Body = httptest.NewRequest("POST", "/", stringsNewReader(body)).Body

	h.CreateAlbum(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var item AlbumItem
	json.Unmarshal(w.Body.Bytes(), &item)
	if item.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	// Cleanup
	h.AlbumSvc.DeleteAlbum(item.ID)
}

func TestUpdateAlbumHandler(t *testing.T) {
	h := setupHandler(t)
	created, _ := h.AlbumSvc.CreateAlbum("H_Update", "")

	body := `{"title":"H_Updated"}`
	c, w := ginCtx("PUT", "/api/v1/albums/"+itoa(created.ID))
	c.Params = gin.Params{{Key: "id", Value: itoa(created.ID)}}
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Body = httptest.NewRequest("PUT", "/", stringsNewReader(body)).Body

	h.UpdateAlbum(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var item AlbumItem
	json.Unmarshal(w.Body.Bytes(), &item)
	if item.Title != "H_Updated" {
		t.Fatalf("title not updated: %s", item.Title)
	}
	h.AlbumSvc.DeleteAlbum(created.ID)
}

func TestDeleteAlbumHandler(t *testing.T) {
	h := setupHandler(t)
	created, _ := h.AlbumSvc.CreateAlbum("H_Delete", "")

	c, w := ginCtx("DELETE", "/api/v1/albums/"+itoa(created.ID))
	c.Params = gin.Params{{Key: "id", Value: itoa(created.ID)}}

	h.DeleteAlbum(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// Verify deleted
	_, err := h.AlbumSvc.GetAlbum(created.ID)
	if err == nil {
		t.Fatal("album should be deleted")
	}
}

func TestBatchAddPhotosHandler(t *testing.T) {
	h := setupHandler(t)
	created, _ := h.AlbumSvc.CreateAlbum("H_Batch", "")
	defer h.AlbumSvc.DeleteAlbum(created.ID)

	var photoIDs []int64
	h.DB.Raw("SELECT id FROM photos LIMIT 3").Scan(&photoIDs)
	body := fmtJSON(`{"photo_ids":[%d,%d,%d]}`, photoIDs[0], photoIDs[1], photoIDs[2])

	c, w := ginCtx("POST", "/api/v1/albums/"+itoa(created.ID)+"/photos")
	c.Params = gin.Params{{Key: "id", Value: itoa(created.ID)}}
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Body = httptest.NewRequest("POST", "/", stringsNewReader(body)).Body

	h.BatchAddPhotos(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	t.Logf("batch add result: %s", w.Body.String())
}

func TestRemoveAlbumPhotoHandler(t *testing.T) {
	h := setupHandler(t)
	created, _ := h.AlbumSvc.CreateAlbum("H_Remove", "")
	defer h.AlbumSvc.DeleteAlbum(created.ID)

	var photoIDs []int64
	h.DB.Raw("SELECT id FROM photos LIMIT 1").Scan(&photoIDs)
	h.AlbumSvc.BatchAddPhotos(created.ID, photoIDs)

	c, w := ginCtx("DELETE", "/api/v1/albums/"+itoa(created.ID)+"/photos/"+itoa(photoIDs[0]))
	c.Params = gin.Params{{Key: "id", Value: itoa(created.ID)}, {Key: "photo_id", Value: itoa(photoIDs[0])}}

	h.RemoveAlbumPhoto(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListAlbumPhotos_MonthFilter(t *testing.T) {
	h := setupHandler(t)
	c, w := ginCtx("GET", "/api/v1/albums/1/photos?month=2025-01&limit=5")
	c.Params = gin.Params{{Key: "id", Value: "1"}}

	h.ListAlbumPhotos(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp AlbumPhotoListResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	t.Logf("album 1, Jan 2025: %d photos, head_count=%d", resp.Total, resp.HeadCount)
}

// ═══════════════════════════════════════════════════════
// Error cases
// ═══════════════════════════════════════════════════════

func TestListPhotos_InvalidLimit(t *testing.T) {
	h := setupHandler(t)
	c, w := ginCtx("GET", "/api/v1/photos?limit=9999")
	h.ListPhotos(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp PhotoListResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	// Should be capped at 200
	if len(resp.Items) > 200 {
		t.Fatalf("expected <=200 items, got %d", len(resp.Items))
	}
}

func TestGetAlbum_InvalidID(t *testing.T) {
	h := setupHandler(t)
	c, w := ginCtx("GET", "/api/v1/albums/abc")
	c.Params = gin.Params{{Key: "id", Value: "abc"}}
	h.GetAlbum(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateAlbum_EmptyTitle(t *testing.T) {
	h := setupHandler(t)
	body := `{"title":""}`
	c, w := ginCtx("POST", "/api/v1/albums")
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Body = httptest.NewRequest("POST", "/", stringsNewReader(body)).Body
	h.CreateAlbum(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBatchAddPhotos_EmptyList(t *testing.T) {
	h := setupHandler(t)
	body := `{"photo_ids":[]}`
	c, w := ginCtx("POST", "/api/v1/albums/1/photos")
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Body = httptest.NewRequest("POST", "/", stringsNewReader(body)).Body
	h.BatchAddPhotos(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestUpdateAlbum_NotFound(t *testing.T) {
	h := setupHandler(t)
	body := `{"title":"x"}`
	c, w := ginCtx("PUT", "/api/v1/albums/99999999")
	c.Params = gin.Params{{Key: "id", Value: "99999999"}}
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Body = httptest.NewRequest("PUT", "/", stringsNewReader(body)).Body
	h.UpdateAlbum(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestListPhotos_DateJump(t *testing.T) {
	h := setupHandler(t)
	gin.SetMode(gin.TestMode)

	// Find a real photo date to jump to
	var latestDate string
	h.DB.Raw("SELECT to_char(taken_at, 'YYYY-MM-DD') FROM photos WHERE taken_at IS NOT NULL ORDER BY taken_at DESC LIMIT 1").Scan(&latestDate)
	if latestDate == "" {
		t.Skip("no photos with taken_at")
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/photos?date="+latestDate+"&limit=50", nil)
	h.ListPhotos(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp PhotoListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Jump to the latest day: total should be the count of photos on/before that day
	if resp.Total == 0 {
		t.Fatal("expected photos for latest-date jump")
	}
	// First item (before any head) must be on/before the target day
	if len(resp.Items) > 0 {
		first := resp.Items[0]
		if first.TakenAt > latestDate+"T23:59:59" {
			t.Fatalf("first photo %s is after target day %s (head should only precede)", first.TakenAt, latestDate)
		}
	}
	t.Logf("date=%s total=%d head_count=%d items=%d", latestDate, resp.Total, resp.HeadCount, len(resp.Items))
}

// helpers
func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

func stringsNewReader(s string) *strings.Reader {
	return strings.NewReader(s)
}

func fmtJSON(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}
