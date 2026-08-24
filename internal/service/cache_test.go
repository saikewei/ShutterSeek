package service

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func newTestCache(t *testing.T) (*Cache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	c := &Cache{Redis: goredis.NewClient(&goredis.Options{Addr: mr.Addr()})}
	return c, mr
}

func TestCacheGetSetJSON(t *testing.T) {
	c, _ := newTestCache(t)
	type payload struct {
		N int
	}
	c.SetJSON("k:v1", payload{N: 42}, time.Minute)
	var got payload
	if !c.GetJSON("k:v1", &got) || got.N != 42 {
		t.Fatalf("got %+v", got)
	}
	var miss payload
	if c.GetJSON("k:miss", &miss) {
		t.Fatal("expected miss")
	}
}

func TestCacheNilRedisNoop(t *testing.T) {
	c := &Cache{Redis: nil}
	c.SetJSON("k", 1, time.Minute)
	if c.GetJSON("k", new(int)) {
		t.Fatal("nil redis must miss")
	}
	c.DelPatterns("cache:first_page:*")
}

func TestCacheDelPatterns(t *testing.T) {
	c, mr := newTestCache(t)
	c.SetJSON("cache:first_page:guest:50", 1, time.Minute)
	c.SetJSON("cache:first_page:50", 2, time.Minute)
	c.SetJSON("cache:albums:guest", 3, time.Minute)
	c.DelPatterns("cache:first_page:*")
	if mr.Exists("cache:first_page:guest:50") || mr.Exists("cache:first_page:50") {
		t.Fatal("first_page keys should be deleted")
	}
	if !mr.Exists("cache:albums:guest") {
		t.Fatal("albums key should remain")
	}
}
