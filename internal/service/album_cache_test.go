package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestListAlbumsCacheByRole(t *testing.T) {
	c, _ := newTestCache(t)
	s := NewAlbumService(nil, c) // 缓存命中时不触库
	s.Cache.SetJSON("cache:albums:", []AlbumItem{{ID: 1, Title: "cached"}}, time.Minute)
	items, err := s.ListAlbums(context.Background(), "admin")
	if err != nil || len(items) != 1 || items[0].Title != "cached" {
		t.Fatalf("cache hit: %v %+v", err, items)
	}
}

func TestListAlbumPhotosPageCacheHit(t *testing.T) {
	c, _ := newTestCache(t)
	s := NewAlbumService(nil, c)
	page := &AlbumPhotoPage{Total: 3}
	data, _ := json.Marshal(page)
	c.Redis.Set(context.Background(), "cache:album_photos:7:50", data, time.Minute)
	got, err := s.ListAlbumPhotosPage(context.Background(), "admin", 7, 50, "", "")
	if err != nil || got.Total != 3 {
		t.Fatalf("cache hit: %v %+v", err, got)
	}
}
