package service

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"shutterseek/internal/model"
)

var (
	ErrEmptyQuery       = errors.New("empty query")
	ErrEmbedUnavailable = errors.New("embed service unavailable")
	ErrEmbedInvalid     = errors.New("embed service invalid response")
)

const (
	qvecCachePrefix = "cache:qvec:v1:"
	qvecCacheTTL    = 7 * 24 * time.Hour
)

type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

type HTTPEmbedder struct {
	url    string
	token  string
	client *http.Client
}

func NewHTTPEmbedder(url string, timeout time.Duration, token string) *HTTPEmbedder {
	return &HTTPEmbedder{
		url:    strings.TrimRight(url, "/"),
		token:  token,
		client: &http.Client{Timeout: timeout},
	}
}

type embedResponse struct {
	Dim    int       `json:"dim"`
	Vector []float32 `json:"vector"`
	Model  string    `json:"model"`
}

func (e *HTTPEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.url+"/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.token != "" {
		req.Header.Set("Authorization", "Bearer "+e.token)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEmbedUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusInternalServerError {
			return nil, fmt.Errorf("%w: status %d", ErrEmbedInvalid, resp.StatusCode)
		}
		return nil, fmt.Errorf("%w: status %d", ErrEmbedUnavailable, resp.StatusCode)
	}
	var out embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEmbedInvalid, err)
	}
	if out.Dim != len(out.Vector) || out.Dim != 1024 {
		return nil, fmt.Errorf("%w: dim=%d len=%d", ErrEmbedInvalid, out.Dim, len(out.Vector))
	}
	return out.Vector, nil
}

type CachedEmbedder struct {
	inner Embedder
	redis *goredis.Client
	ttl   time.Duration
}

func NewCachedEmbedder(inner Embedder, redis *goredis.Client) *CachedEmbedder {
	return &CachedEmbedder{inner: inner, redis: redis, ttl: qvecCacheTTL}
}

func queryCacheKey(q string) string {
	sum := sha1.Sum([]byte(strings.ToLower(strings.TrimSpace(q))))
	return qvecCachePrefix + hex.EncodeToString(sum[:])
}

func (c *CachedEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	key := queryCacheKey(text)
	if c.redis != nil {
		if raw, err := c.redis.Get(ctx, key).Bytes(); err == nil {
			var v []float32
			if json.Unmarshal(raw, &v) == nil && len(v) == 1024 {
				return v, nil
			}
		}
	}
	v, err := c.inner.Embed(ctx, text)
	if err != nil {
		return nil, err
	}
	if c.redis != nil {
		if data, err := json.Marshal(v); err == nil {
			c.redis.Set(ctx, key, data, c.ttl)
		}
	}
	return v, nil
}

func formatVector(v []float32) string {
	var b strings.Builder
	b.Grow(len(v) * 12)
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

type SearchItem struct {
	model.Photo
	Score float64 `gorm:"column:score" json:"score"`
}

type SearchService struct {
	DB       *gorm.DB
	Embedder Embedder
	MaxText  int
}

func NewSearchService(db *gorm.DB, embedder Embedder, maxText int) *SearchService {
	return &SearchService{DB: db, Embedder: embedder, MaxText: maxText}
}

func (s *SearchService) Search(ctx context.Context, q, role string, limit int, albumID int64) ([]SearchItem, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, ErrEmptyQuery
	}
	runes := []rune(q)
	if s.MaxText > 0 && len(runes) > s.MaxText {
		q = string(runes[:s.MaxText])
	}
	vec, err := s.Embedder.Embed(ctx, q)
	if err != nil {
		return nil, err
	}
	vecStr := formatVector(vec)

	query := `SELECT p.*, 1 - (pe.embedding <=> ?::vector) AS score
		FROM photo_embeddings pe
		JOIN photos p ON p.id = pe.photo_id
		WHERE TRUE`
	args := []any{vecStr}
	if albumID > 0 {
		query += ` AND p.id IN (SELECT photo_id FROM album_photos WHERE album_id = ?)`
		args = append(args, albumID)
	}
	if role == "guest" {
		query += ` AND p.id IN (SELECT ap.photo_id FROM album_photos ap
			JOIN albums a ON a.id = ap.album_id WHERE a.is_public = true)`
	}
	query += ` ORDER BY pe.embedding <=> ?::vector LIMIT ?`
	args = append(args, vecStr, limit)

	var items []SearchItem
	// enable_sort=off：强制规划器使用 HNSW 索引的按距离有序扫描（而非先物化
	// 过滤集合再精确排序）。对 guest/相册等带过滤查询，避免退化为全量精确扫描
	//（实测 guest 从 ~450ms 降至 ~1ms）。SET LOCAL 仅在本次事务生效。
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL enable_sort = off").Error; err != nil {
			return err
		}
		return tx.Raw(query, args...).Scan(&items).Error
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}
