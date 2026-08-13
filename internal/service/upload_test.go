package service

import (
	"image"
	"image/color"
	"testing"
)

func TestErrDuplicate(t *testing.T) {
	e := ErrDuplicate{ExistingID: 42}
	if e.Error() != "duplicate photo id=42" {
		t.Fatalf("err: %s", e.Error())
	}
}

func TestResizeShort(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4000, 2000))
	img.Set(0, 0, color.White)
	got := resizeShort(img, 1080)
	b := got.Bounds()
	if b.Dx() != 2160 || b.Dy() != 1080 {
		t.Fatalf("resized to %dx%d", b.Dx(), b.Dy())
	}
}

func TestUploadAbsPath(t *testing.T) {
	if got := uploadAbsPath("/photos_uploads", "uploads/2026/08/a.jpg"); got != "/photos_uploads/2026/08/a.jpg" {
		t.Fatalf("got %q", got)
	}
	if got := uploadAbsPath("/photos_uploads", "2026/08/a.jpg"); got != "/photos_uploads/2026/08/a.jpg" {
		t.Fatalf("no-prefix got %q", got)
	}
}
