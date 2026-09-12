package service

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

// 窗口内多次上传只清一次缓存（现状是每张一次 SCAN）。
func TestCacheInvalidatorThrottlesWithinWindow(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	ctx := context.Background()

	rdb.Set(ctx, KeyTotalPhotos, "1", time.Minute)
	rdb.Set(ctx, KeyFirstPage+"a", "1", time.Minute)
	rdb.Set(ctx, "cache:photo_dates:guest", "1", time.Minute)

	v := NewCacheInvalidator(&Cache{Redis: rdb}, time.Hour) // 窗口足够长，确保不会自动补清
	for i := 0; i < 20; i++ {
		v.Request()
	}

	if n := rdb.Exists(ctx, KeyTotalPhotos).Val(); n != 0 {
		t.Fatalf("第一次 Request 应立即清掉总数键，exists=%d", n)
	}
	// 第一次立即清空后，窗口内的 19 次只置 dirty，不再重复清
	rdb.Set(ctx, KeyTotalPhotos, "2", time.Minute)
	for i := 0; i < 5; i++ {
		v.Request()
	}
	if n := rdb.Exists(ctx, KeyTotalPhotos).Val(); n != 1 {
		t.Fatalf("窗口内不应重复清理，exists=%d", n)
	}

	// FlushNow 补清未落地的失效
	v.FlushNow()
	if n := rdb.Exists(ctx, KeyTotalPhotos).Val(); n != 0 {
		t.Fatalf("FlushNow 应清掉 dirty 的键，exists=%d", n)
	}
	if n := rdb.Exists(ctx, KeyFirstPage+"a").Val(); n != 0 {
		t.Fatalf("first_page 也应被清，exists=%d", n)
	}
}

// 后台 ticker 必须在窗口边界把 dirty 的失效补上。
func TestCacheInvalidatorTickerFlushes(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	ctx := context.Background()

	v := NewCacheInvalidator(&Cache{Redis: rdb}, 20*time.Millisecond)
	v.Request() // 首次立即清（此时没有键）
	rdb.Set(ctx, KeyTotalPhotos, "1", time.Minute)
	v.Request() // 窗口内 → 只置 dirty
	v.Start()
	defer v.Stop()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if rdb.Exists(ctx, KeyTotalPhotos).Val() == 0 {
			return // ticker 补清成功
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("ticker 未在窗口边界补清缓存")
}

// Redis 缺失时（dev/降级）必须安全 no-op。
func TestCacheInvalidatorNilRedis(t *testing.T) {
	v := NewCacheInvalidator(&Cache{}, time.Millisecond)
	v.Request()
	v.Start()
	v.Request()
	v.FlushNow()
	v.Stop()
}

// 零值 interval 归一化成默认窗口。
func TestCacheInvalidatorDefaultInterval(t *testing.T) {
	if got := NewCacheInvalidator(nil, 0).interval; got != DefaultInvalidateInterval {
		t.Fatalf("interval=%v", got)
	}
}
