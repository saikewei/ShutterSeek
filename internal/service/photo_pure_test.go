package service

import (
	"testing"
	"time"
)

func TestParseCursor(t *testing.T) {
	tm, id, ok := parseCursor("2026-08-13T15:04:05,42")
	if !ok || id != 42 || tm.Format("2006-01-02T15:04:05") != "2026-08-13T15:04:05" {
		t.Fatalf("got %v %d %v", tm, id, ok)
	}
	if _, _, ok := parseCursor("garbage"); ok {
		t.Fatal("expected invalid")
	}
}

func TestBuildNextCursor(t *testing.T) {
	if got := buildNextCursor(time.Date(2026, 8, 13, 15, 4, 5, 0, cstZone), 7); got != "2026-08-13T15:04:05,7" {
		t.Fatalf("got %q", got)
	}
	if got := buildNextCursor(time.Time{}, 7); got != "0001-01-01T00:00:00,7" {
		t.Fatalf("zero got %q", got)
	}
}
