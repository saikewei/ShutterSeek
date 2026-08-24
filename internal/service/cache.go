package service

import (
	"context"
	"encoding/json"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// 缓存 key/TTL（与重构前 handler 层一致，逐字保留）
const (
	KeyTotalPhotos = "cache:total_photos"
	KeyFirstPage   = "cache:first_page:"
	TTLTotal       = 5 * time.Minute
	TTLFirstPage   = 60 * time.Second
	TTLAlbums      = 60 * time.Second
	TTLDates       = 5 * time.Minute
)

// Cache 封装 Redis 读写；Redis 为 nil 时全部安全降级（开发/无缓存环境）。
type Cache struct {
	Redis *goredis.Client
}

func (c *Cache) GetJSON(key string, dst any) bool {
	if c.Redis == nil {
		return false
	}
	data, err := c.Redis.Get(context.Background(), key).Bytes()
	if err != nil {
		return false
	}
	return json.Unmarshal(data, dst) == nil
}

func (c *Cache) SetJSON(key string, v any, ttl time.Duration) {
	if c.Redis == nil {
		return
	}
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	c.Redis.Set(context.Background(), key, data, ttl)
}

func (c *Cache) GetBytes(key string) ([]byte, bool) {
	if c.Redis == nil {
		return nil, false
	}
	data, err := c.Redis.Get(context.Background(), key).Bytes()
	if err != nil {
		return nil, false
	}
	return data, true
}

func (c *Cache) Del(keys ...string) {
	if c.Redis == nil || len(keys) == 0 {
		return
	}
	c.Redis.Del(context.Background(), keys...)
}

// DelPatterns 按模式 SCAN+DEL；任何一轮出错即停止（与旧 clear* 行为一致）。
func (c *Cache) DelPatterns(patterns ...string) {
	if c.Redis == nil {
		return
	}
	ctx := context.Background()
	for _, pat := range patterns {
		var cursor uint64
		for {
			keys, next, err := c.Redis.Scan(ctx, cursor, pat, 100).Result()
			if err != nil {
				break
			}
			if len(keys) > 0 {
				c.Redis.Del(ctx, keys...)
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
}
