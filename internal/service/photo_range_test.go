package service

import (
	"testing"
	"time"
)

func TestEffectiveTakenAt(t *testing.T) {
	if got := effectiveTakenAt(time.Time{}); !got.Equal(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("zero -> %v", got)
	}
	ts := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	if got := effectiveTakenAt(ts); !got.Equal(ts) {
		t.Fatalf("non-zero -> %v", got)
	}
}

func TestPhotoTupleLess(t *testing.T) {
	a := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	b := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	if !photoTupleLess(a, 1, b, 2) {
		t.Fatal("earlier time should sort first")
	}
	if !photoTupleLess(a, 1, a, 2) {
		t.Fatal("smaller id should sort first on tie")
	}
	if photoTupleLess(b, 1, a, 1) {
		t.Fatal("later time must not sort first")
	}
}
