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
