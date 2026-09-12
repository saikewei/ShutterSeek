package service

import (
	"sync"
	"time"
)

// DefaultInvalidateInterval 是上传后缓存失效的合并窗口：窗口内多次请求只清一次。
const DefaultInvalidateInterval = 2 * time.Second

// CacheInvalidator 把「每次上传都 SCAN+DEL 一遍」合并成「窗口内最多一次」。
//
// 语义：Request 距上次清理 ≥ interval 时立即清（保证调用方紧接着的读不会看到
// 陈旧数据），否则只置 dirty，由后台 ticker 在窗口边界补清。最坏陈旧度 ≈ interval。
type CacheInvalidator struct {
	cache    *Cache
	interval time.Duration

	mu      sync.Mutex
	last    time.Time
	dirty   bool
	started bool
	stop    chan struct{}
	done    chan struct{}
}

func NewCacheInvalidator(cache *Cache, interval time.Duration) *CacheInvalidator {
	if interval <= 0 {
		interval = DefaultInvalidateInterval
	}
	return &CacheInvalidator{cache: cache, interval: interval}
}

// Request 登记一次「上传改变了照片集合」，必要时立即清理。
func (v *CacheInvalidator) Request() {
	if v == nil {
		return
	}
	v.mu.Lock()
	now := time.Now()
	if v.last.IsZero() || now.Sub(v.last) >= v.interval {
		v.last = now
		v.dirty = false
		v.mu.Unlock()
		v.purge()
		return
	}
	v.dirty = true
	v.mu.Unlock()
}

// Start 启动后台补清 ticker（幂等；未 Start 时 Request 仍按窗口立即清理）。
func (v *CacheInvalidator) Start() {
	if v == nil {
		return
	}
	v.mu.Lock()
	if v.started {
		v.mu.Unlock()
		return
	}
	v.started = true
	v.stop = make(chan struct{})
	v.done = make(chan struct{})
	stop, done, interval := v.stop, v.done, v.interval
	v.mu.Unlock()

	go func() {
		defer close(done)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				v.tick()
			}
		}
	}()
}

// Stop 停止后台 ticker（不清理）。
func (v *CacheInvalidator) Stop() {
	if v == nil {
		return
	}
	v.mu.Lock()
	if !v.started {
		v.mu.Unlock()
		return
	}
	v.started = false
	stop, done := v.stop, v.done
	v.mu.Unlock()
	close(stop)
	<-done
}

// FlushNow 立刻清理未落地的失效请求（进程退出前用）。
func (v *CacheInvalidator) FlushNow() {
	if v == nil {
		return
	}
	v.mu.Lock()
	pending := v.dirty
	v.dirty = false
	v.last = time.Now()
	v.mu.Unlock()
	if pending {
		v.purge()
	}
}

func (v *CacheInvalidator) tick() {
	v.mu.Lock()
	if !v.dirty {
		v.mu.Unlock()
		return
	}
	v.dirty = false
	v.last = time.Now()
	v.mu.Unlock()
	v.purge()
}

// purge 清除上传后受影响的所有缓存键（总数按角色分开，用模式一次清掉）。
func (v *CacheInvalidator) purge() {
	if v.cache == nil {
		return
	}
	v.cache.DelPatterns(KeyTotalPhotos+"*", KeyFirstPage+"*", "cache:photo_dates*")
}
