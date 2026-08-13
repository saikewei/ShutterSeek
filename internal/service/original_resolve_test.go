package service

import "testing"

func TestResolvePath(t *testing.T) {
	s := NewOriginalService("/photos", "/photos_uploads", "/tmp/previews")
	if got := s.resolvePath("film/ektar100/1 002.tiff"); got != "/photos/film/ektar100/1 002.tiff" {
		t.Fatalf("library path: %s", got)
	}
	if got := s.resolvePath("uploads/2026/08/a.jpg"); got != "/photos_uploads/2026/08/a.jpg" {
		t.Fatalf("upload path: %s", got)
	}
}
