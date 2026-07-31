//go:build integration

package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"shutterseek/internal/model"
)

// insertRangePhotos creates n photos with taken_at spaced 1h apart starting
// 2020-06-01, returns them, and registers cleanup. Using 2020 dates keeps the
// interval disjoint from real (2024+) photo data.
func insertRangePhotos(t *testing.T, h *Handler, n int) []model.Photo {
	t.Helper()
	base := time.Date(2020, 6, 1, 12, 0, 0, 0, time.UTC)
	prefix := fmt.Sprintf("TEST_RANGE_%d_", time.Now().UnixNano())
	photos := make([]model.Photo, n)
	for i := 0; i < n; i++ {
		photos[i] = model.Photo{
			FilePath: fmt.Sprintf("%s%d.jpg", prefix, i),
			FileHash: fmt.Sprintf("hash%d", i),
			TakenAt:  base.Add(time.Duration(i) * time.Hour),
			Status:   1,
		}
	}
	if err := h.DB.Create(&photos).Error; err != nil {
		t.Fatalf("insert photos: %v", err)
	}
	t.Cleanup(func() {
		ids := make([]int64, len(photos))
		for i, p := range photos {
			ids[i] = p.ID
		}
		h.DB.Where("id IN ?", ids).Delete(&model.Photo{})
	})
	return photos
}

func doRange(t *testing.T, h *Handler, query string) (int, []int64) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/photos/range"+query, nil)
	h.PhotoRange(c)
	var body struct {
		PhotoIDs []int64 `json:"photo_ids"`
		Count    int     `json:"count"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body.PhotoIDs
}

func idSet(ids []int64) map[int64]bool {
	m := make(map[int64]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func TestPhotoRange_ForwardInclusive(t *testing.T) {
	h := setupHandler(t)
	photos := insertRangePhotos(t, h, 10)

	code, ids := doRange(t, h, fmt.Sprintf("?from_id=%d&to_id=%d", photos[0].ID, photos[9].ID))
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if len(ids) != 10 {
		t.Fatalf("expected 10 ids, got %d: %v", len(ids), ids)
	}
	// Both endpoints included
	got := idSet(ids)
	if !got[photos[0].ID] || !got[photos[9].ID] {
		t.Fatal("endpoints must be included")
	}
	// Descending order (newest first)
	for i := 1; i < len(ids); i++ {
		if ids[i] > ids[i-1] {
			t.Fatalf("not descending: %d before %d", ids[i], ids[i-1])
		}
	}
}

func TestPhotoRange_Reverse(t *testing.T) {
	h := setupHandler(t)
	photos := insertRangePhotos(t, h, 10)

	code, ids := doRange(t, h, fmt.Sprintf("?from_id=%d&to_id=%d", photos[9].ID, photos[0].ID))
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if len(ids) != 10 {
		t.Fatalf("reverse range should match forward: got %d", len(ids))
	}
}

func TestPhotoRange_SubInterval(t *testing.T) {
	h := setupHandler(t)
	photos := insertRangePhotos(t, h, 10)

	code, ids := doRange(t, h, fmt.Sprintf("?from_id=%d&to_id=%d", photos[2].ID, photos[7].ID))
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if len(ids) != 6 {
		t.Fatalf("expected 6 ids (photos 2..7), got %d", len(ids))
	}
}

func TestPhotoRange_AlbumScoped(t *testing.T) {
	h := setupHandler(t)
	photos := insertRangePhotos(t, h, 6)
	album, err := h.AlbumSvc.CreateAlbum("TEST_RANGE_ALBUM", "")
	if err != nil {
		t.Fatalf("create album: %v", err)
	}
	t.Cleanup(func() { _ = h.AlbumSvc.DeleteAlbum(album.ID) })
	// Add photos[1..4] (4 photos) to the album
	if _, err := h.AlbumSvc.BatchAddPhotos(album.ID, []int64{photos[1].ID, photos[2].ID, photos[3].ID, photos[4].ID}); err != nil {
		t.Fatalf("batch add: %v", err)
	}

	code, ids := doRange(t, h, fmt.Sprintf("?from_id=%d&to_id=%d&album_id=%d", photos[0].ID, photos[5].ID, album.ID))
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if len(ids) != 4 {
		t.Fatalf("expected 4 album-scoped ids, got %d: %v", len(ids), ids)
	}
}

func TestPhotoRange_Uncategorized(t *testing.T) {
	h := setupHandler(t)
	photos := insertRangePhotos(t, h, 6)
	album, _ := h.AlbumSvc.CreateAlbum("TEST_RANGE_UC", "")
	t.Cleanup(func() { _ = h.AlbumSvc.DeleteAlbum(album.ID) })
	// Categorize 4 of them; 2 remain uncategorized
	h.AlbumSvc.BatchAddPhotos(album.ID, []int64{photos[0].ID, photos[1].ID, photos[2].ID, photos[3].ID})

	code, ids := doRange(t, h, fmt.Sprintf("?from_id=%d&to_id=%d&album_id=none", photos[0].ID, photos[5].ID))
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 uncategorized ids, got %d: %v", len(ids), ids)
	}
}

func TestPhotoRange_InvalidIDs(t *testing.T) {
	h := setupHandler(t)
	// Non-numeric
	if code, _ := doRange(t, h, "?from_id=abc&to_id=1"); code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-numeric, got %d", code)
	}
	// Missing to_id
	if code, _ := doRange(t, h, "?from_id=1"); code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing to_id, got %d", code)
	}
	// Anchor not found
	if code, _ := doRange(t, h, "?from_id=999999999&to_id=999999998"); code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing anchors, got %d", code)
	}
}

func TestPhotoRange_OverLimit(t *testing.T) {
	h := setupHandler(t)
	photos := insertRangePhotos(t, h, rangeSelectLimit+1) // 5001

	code, _ := doRange(t, h, fmt.Sprintf("?from_id=%d&to_id=%d", photos[0].ID, photos[rangeSelectLimit].ID))
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 over limit, got %d", code)
	}
}
