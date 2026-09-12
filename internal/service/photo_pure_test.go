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
	if got := BuildNextCursor(time.Date(2026, 8, 13, 15, 4, 5, 0, cstZone), 7); got != "2026-08-13T15:04:05,7" {
		t.Fatalf("got %q", got)
	}
	if got := BuildNextCursor(time.Time{}, 7); got != "0001-01-01T00:00:00,7" {
		t.Fatalf("zero got %q", got)
	}
}

// 回归（北京2025 相册只显示 302/826）：taken_at 从 pgx 扫出来是 UTC，
// 游标必须以 +08 墙钟写出，否则被 parseCursor 按 +08 解析后整体前移 8 小时，
// 下一页的 `(taken_at, id) < ?` 会跳过当天剩余的全部照片。
func TestBuildNextCursor_UTCInputIsWrittenInCST(t *testing.T) {
	utc := time.Date(2025, 1, 14, 15, 16, 7, 0, time.UTC) // 同一瞬间的 +08 墙钟为 23:16:07
	if got, want := BuildNextCursor(utc, 64968), "2025-01-14T23:16:07,64968"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// 游标编解码必须无损：无论输入时间是 UTC 还是 +08，往返后必须落在同一瞬间。
func TestCursorRoundTripKeepsInstant(t *testing.T) {
	for _, loc := range []*time.Location{time.UTC, cstZone} {
		orig := time.Date(2025, 1, 14, 23, 16, 7, 0, loc)
		tm, id, ok := parseCursor(BuildNextCursor(orig, 42))
		if !ok || id != 42 {
			t.Fatalf("loc=%v parse failed: %v %d %v", loc, tm, id, ok)
		}
		if !tm.Equal(orig) {
			t.Fatalf("loc=%v instant drift: got %v want %v", loc, tm, orig)
		}
	}
}

// NULL 尾部队列：BuildNextCursor 写固定哨兵，parseCursor 必须把它还原成零值时间，
// 调用方才能据此走 `taken_at IS NULL AND id < ?` 分支继续翻页。
func TestCursorNullTailSentinel(t *testing.T) {
	cur := BuildNextCursor(time.Time{}, 42)
	if cur != "0001-01-01T00:00:00,42" {
		t.Fatalf("哨兵格式变了: %q", cur)
	}
	tm, id, ok := parseCursor(cur)
	if !ok || id != 42 || !tm.IsZero() {
		t.Fatalf("哨兵未被还原成零值时间: t=%v id=%d ok=%v", tm, id, ok)
	}
}
