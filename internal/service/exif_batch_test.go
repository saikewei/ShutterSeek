package service

import (
	"testing"
	"time"
)

// parseEXIFMany 必须按 SourceFile 逐条归位，且不依赖 exiftool 进程。
func TestParseEXIFMany(t *testing.T) {
	records := []map[string]any{
		{
			"SourceFile":       "/tmp/.upload-aaa",
			"ImageWidth":       float64(7008),
			"ImageHeight":      float64(4672),
			"Make":             "SONY",
			"Model":            "ILCE-7M3",
			"LensModel":        "FE 24-70mm F2.8 GM",
			"FNumber":          float64(2.8),
			"ISO":              float64(400),
			"FocalLength":      "35.0 mm",
			"DateTimeOriginal": "2024:05:01 10:20:30",
		},
		{
			"SourceFile":  "/tmp/.upload-bbb",
			"ImageWidth":  float64(4032),
			"ImageHeight": float64(3024),
			// 没有拍摄时间 → 用 fallback
		},
	}

	fallback := time.Date(2026, 1, 2, 3, 4, 5, 0, cstZone)
	got := parseEXIFMany(records, func(string) time.Time { return fallback })

	if len(got) != 2 {
		t.Fatalf("want 2 records, got %d", len(got))
	}

	a := got["/tmp/.upload-aaa"]
	if a == nil {
		t.Fatal("缺 a 记录")
	}
	if a.Width != 7008 || a.Height != 4672 {
		t.Fatalf("a 尺寸: %dx%d", a.Width, a.Height)
	}
	if a.CameraMake != "SONY" || a.CameraModel != "ILCE-7M3" || a.LensModel != "FE 24-70mm F2.8 GM" {
		t.Fatalf("a 相机信息: %+v", a)
	}
	if a.Aperture != 2.8 || a.ISO != 400 || a.FocalLength != 35 {
		t.Fatalf("a 曝光信息: %+v", a)
	}
	want := time.Date(2024, 5, 1, 10, 20, 30, 0, cstZone)
	if a.TakenAt == nil || !a.TakenAt.Equal(want) {
		t.Fatalf("a 拍摄时间: %v want %v", a.TakenAt, want)
	}

	b := got["/tmp/.upload-bbb"]
	if b == nil || b.TakenAt == nil || !b.TakenAt.Equal(fallback) {
		t.Fatalf("b 应回落到 fallback: %+v", b)
	}
}

// 没有 SourceFile 的记录直接丢弃，不污染其它项。
func TestParseEXIFManySkipsSourceless(t *testing.T) {
	got := parseEXIFMany([]map[string]any{
		{"ImageWidth": float64(100)},
		{"SourceFile": "/x", "ImageWidth": float64(200)},
	}, nil)
	if len(got) != 1 {
		t.Fatalf("want 1 record, got %d: %+v", len(got), got)
	}
	if got["/x"] == nil || got["/x"].Width != 200 {
		t.Fatalf("归位错误: %+v", got)
	}
}
