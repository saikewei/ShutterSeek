//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"shutterseek/internal/service"
)

type stubEmbedder struct {
	vec []float32
	err error
}

func (s *stubEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return s.vec, s.err
}

func setupSearchHandler(t *testing.T) *Handler {
	t.Helper()
	h := setupHandler(t)
	vec := make([]float32, 1024)
	for i := range vec {
		vec[i] = 0.03125
	}
	h.SearchSvc = service.NewSearchService(h.DB, &stubEmbedder{vec: vec}, 200)
	return h
}

func doSearch(t *testing.T, h *Handler, q string, role string) (*httptest.ResponseRecorder, gin.H) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/search?q="+q, nil)
	c.Set("role", role)
	h.Search(c)
	var body gin.H
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body: %s", w.Body.String())
	}
	return w, body
}

func TestSearch_ReturnsItems(t *testing.T) {
	h := setupSearchHandler(t)
	w, body := doSearch(t, h, "%E6%B5%B7%E8%BE%B9", "admin")
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d body = %s", w.Code, w.Body.String())
	}
	items, ok := body["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("no items: %v", body)
	}
	first := items[0].(map[string]any)
	if _, ok := first["score"]; !ok {
		t.Fatalf("item missing score: %v", first)
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	h := setupSearchHandler(t)
	w, _ := doSearch(t, h, "", "admin")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("code = %d", w.Code)
	}
}

func TestSearch_EmbedUnavailable(t *testing.T) {
	h := setupHandler(t)
	h.SearchSvc = service.NewSearchService(h.DB, &stubEmbedder{err: service.ErrEmbedUnavailable}, 200)
	w, _ := doSearch(t, h, "x", "admin")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d body = %s", w.Code, w.Body.String())
	}
}

func TestSearch_GuestForbiddenAlbum(t *testing.T) {
	h := setupSearchHandler(t)
	var albumID int64
	h.DB.Raw(`SELECT id FROM albums WHERE is_public = false ORDER BY id LIMIT 1`).Scan(&albumID)
	if albumID == 0 {
		t.Skip("no private album found")
	}
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/search?q=x&album_id="+itoa(albumID), nil)
	c.Set("role", "guest")
	h.Search(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("code = %d", w.Code)
	}
}
