package service

import (
	"strings"
	"testing"
	"time"
)

func TestSanitizeBaseName(t *testing.T) {
	cases := map[string]string{
		"IMG_0123.JPG":          "IMG_0123.JPG",
		"../evil/../../a b.jpg": "a_b.jpg",
		"照片 001.NEF":            "___001.NEF",
	}
	for in, want := range cases {
		if got := sanitizeBaseName(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildUploadRelPath(t *testing.T) {
	ts := time.Date(2026, 8, 13, 15, 30, 0, 0, cstZone)
	got := buildUploadRelPath(ts, "IMG_0123.JPG", "")
	if got != "uploads/2026/08/20260813-153000_IMG_0123.JPG" {
		t.Fatalf("got %q", got)
	}
	got = buildUploadRelPath(ts, "IMG_0123.JPG", "a1b2c3d4")
	if !strings.HasSuffix(got, "20260813-153000_IMG_0123_a1b2c3d4.JPG") {
		t.Fatalf("collision name got %q", got)
	}
}
