package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

// A body that starts with the RIFF/WEBP signature, so Content-Type resolves the
// same way whether Go takes it from the extension or from content sniffing.
var webpBody = []byte("RIFF\x00\x00\x00\x00WEBPVP8 " + "test-payload")

// newThumbRouter mounts Thumbnail behind a stub that just sets the role, which
// keeps these tests off the database: only the guest branch needs PhotoSvc.
func newThumbRouter(dir, role string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := &Handler{ThumbnailsDir: dir}
	r := gin.New()
	r.GET("/thumbnails/:file", func(c *gin.Context) {
		c.Set("role", role)
	}, h.Thumbnail)
	return r
}

func writeThumb(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), webpBody, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestThumbnailServesFile(t *testing.T) {
	dir := t.TempDir()
	writeThumb(t, dir, "42.webp")

	w := httptest.NewRecorder()
	newThumbRouter(dir, "admin").ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/thumbnails/42.webp", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Body.String() != string(webpBody) {
		t.Fatalf("body = %q, want the file contents", w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/webp" {
		t.Errorf("content-type = %q, want image/webp", ct)
	}
	// Authenticated content must not enter shared caches.
	if cc := w.Header().Get("Cache-Control"); cc != "private, max-age=86400" {
		t.Errorf("cache-control = %q, want private, max-age=86400", cc)
	}
}

func TestThumbnailMissingFile(t *testing.T) {
	w := httptest.NewRecorder()
	newThumbRouter(t.TempDir(), "admin").ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/thumbnails/7.webp", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestThumbnailRejectsBadNames(t *testing.T) {
	dir := t.TempDir()
	writeThumb(t, dir, "42.webp")

	router := newThumbRouter(dir, "admin")
	// These reach the handler and must be refused by the strict name parser.
	for _, name := range []string{"42.png", "abc.webp", "42.webp.webp", "0.webp", "-1.webp"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/thumbnails/"+name, nil))
		if w.Code != http.StatusBadRequest {
			t.Errorf("name %q: status = %d, want 400", name, w.Code)
		}
	}

	// Traversal attempts are refused even earlier, by the router itself, so
	// they must simply never produce a file.
	for _, name := range []string{"..%2f42.webp", "%2e%2e%2f42.webp"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/thumbnails/"+name, nil))
		if w.Code == http.StatusOK {
			t.Errorf("name %q: served %d bytes, want a refusal", name, w.Body.Len())
		}
	}
}

// The guest branch is the one that consults the database; with no PhotoSvc it
// must fail closed rather than serve the file.
func TestThumbnailGuestWithNoServiceFailsClosed(t *testing.T) {
	dir := t.TempDir()
	writeThumb(t, dir, "42.webp")

	w := httptest.NewRecorder()
	newThumbRouter(dir, "guest").ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/thumbnails/42.webp", nil))

	if w.Code == http.StatusOK {
		t.Fatal("a guest request must never be served without the visibility check")
	}
}
