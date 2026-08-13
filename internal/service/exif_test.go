package service

import (
	"testing"
	"time"
)

func TestParseEXIF(t *testing.T) {
	m := map[string]any{
		"DateTimeOriginal": "2026:08:13 15:30:00",
		"ImageWidth":       float64(4000),
		"ImageHeight":      float64(3000),
		"Make":             "Apple",
		"Model":            "iPhone 15",
		"FNumber":          float64(1.8),
		"ISO":              float64(100),
		"FocalLength":      "4.2 mm",
	}
	fallback := time.Date(2026, 1, 1, 0, 0, 0, 0, cstZone)
	ex := parseEXIF(m, fallback)
	if ex.TakenAt == nil || ex.TakenAt.Format("2006-01-02 15:04:05") != "2026-08-13 15:30:00" {
		t.Fatalf("taken_at=%v", ex.TakenAt)
	}
	if ex.Width != 4000 || ex.Height != 3000 || ex.CameraMake != "Apple" || ex.ISO != 100 {
		t.Fatalf("fields: %+v", ex)
	}
	if ex.FocalLength != 4.2 || ex.Aperture != 1.8 {
		t.Fatalf("numbers: %+v", ex)
	}
}

func TestParseEXIFFallback(t *testing.T) {
	fallback := time.Date(2026, 3, 4, 5, 6, 7, 0, cstZone)
	ex := parseEXIF(map[string]any{"ImageWidth": float64(1)}, fallback)
	if ex.TakenAt == nil || !ex.TakenAt.Equal(fallback) {
		t.Fatalf("expected fallback, got %v", ex.TakenAt)
	}
}
