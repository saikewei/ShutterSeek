package service

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func TestThumbResizeArgs(t *testing.T) {
	cases := []struct {
		name     string
		w, h     int
		want     []string
		wantNone bool
	}{
		{name: "横图缩宽", w: 7008, h: 4672, want: []string{"-resize", "1080", "0"}},
		{name: "竖图缩高", w: 4672, h: 7008, want: []string{"-resize", "0", "1080"}},
		{name: "方图缩高（与缩宽等价）", w: 5814, h: 5814, want: []string{"-resize", "0", "1080"}},
		{name: "横图长边正好 1080 不缩放", w: 1080, h: 720, wantNone: true},
		{name: "竖图长边正好 1080 不缩放", w: 720, h: 1080, wantNone: true},
		{name: "小横图不放大", w: 800, h: 600, wantNone: true},
		{name: "小竖图不放大", w: 600, h: 800, wantNone: true},
		{name: "尺寸未知不缩放", w: 0, h: 0, wantNone: true},
		{name: "只有宽不缩放", w: 4000, h: 0, wantNone: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := thumbResizeArgs(tc.w, tc.h, thumbTarget)
			if tc.wantNone {
				if len(got) != 0 {
					t.Fatalf("want no resize, got %v", got)
				}
				return
			}
			if len(got) != len(tc.want) {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("want %v, got %v", tc.want, got)
				}
			}
		})
	}
}

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.White)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestValidateClientPreview(t *testing.T) {
	good := jpegBytes(t, 1080, 720)
	if !validateClientPreview(good, ".jpg", 7008, 4672) {
		t.Fatal("合法预览应通过")
	}
	if validateClientPreview(nil, ".jpg", 7008, 4672) {
		t.Fatal("空预览应拒绝")
	}
	if validateClientPreview([]byte("not an image"), ".jpg", 7008, 4672) {
		t.Fatal("非图片应拒绝")
	}
	if validateClientPreview(jpegBytes(t, 100, 80), ".jpg", 7008, 4672) {
		t.Fatal("过小应拒绝")
	}
	if validateClientPreview(jpegBytes(t, 2000, 1500), ".jpg", 7008, 4672) {
		t.Fatal("过大应拒绝")
	}
	// 非 RAW：纵横比必须吻合（这里 1080x1080 vs 3:2 传感器）
	if validateClientPreview(jpegBytes(t, 1080, 1080), ".jpg", 7008, 4672) {
		t.Fatal("纵横比不符应拒绝")
	}
	// RAW：内嵌预览常与传感器比例不同 → 豁免
	if !validateClientPreview(jpegBytes(t, 1080, 1080), ".nef", 7008, 4672) {
		t.Fatal("RAW 应豁免纵横比校验")
	}
	// EXIF 尺寸未知（0）时不做比例判断
	if !validateClientPreview(jpegBytes(t, 1080, 1080), ".jpg", 0, 0) {
		t.Fatal("尺寸未知时应放宽")
	}
}

func TestIsRawExt(t *testing.T) {
	for _, ext := range []string{".nef", ".NEF", ".cr3", ".arw", ".dng", ".raf"} {
		if !isRawExt(ext) {
			t.Fatalf("%s 应识别为 RAW", ext)
		}
	}
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".heic", ".tif", ".webp", ""} {
		if isRawExt(ext) {
			t.Fatalf("%s 不应识别为 RAW", ext)
		}
	}
}
