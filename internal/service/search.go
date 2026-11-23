package service

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"shutterseek/internal/model"
)

var (
	ErrEmptyQuery       = errors.New("empty query")
	ErrInvalidDateRange = errors.New("invalid date range")
	ErrEmbedUnavailable = errors.New("embed service unavailable")
	ErrEmbedInvalid     = errors.New("embed service invalid response")
)

const (
	qvecCachePrefix = "cache:qvec:v1:"
	qvecCacheTTL    = 7 * 24 * time.Hour

	defaultSearchLimit      = 100
	maxSearchLimit          = 200
	defaultExactThreshold   = int64(3000)
	hnswMinEfSearch         = 100
	hnswMaxEfSearch         = 400
	hnswDefaultMaxScanTuple = 20000
	schemaRecheckInterval   = time.Minute

	SearchStrategyEmptyScope   = "empty_scope"
	SearchStrategyTimeSample   = "time_sample"
	SearchStrategyExactHalfvec = "exact_halfvec"
	SearchStrategyHNSWPublicHV = "hnsw_public_hv"
	SearchStrategyHNSWAllHV    = "hnsw_all_hv"
	SearchStrategyHNSWFallback = "hnsw_fallback_exact_hv"
	SearchStrategyLegacyExact  = "legacy_exact"
	SearchStrategyLegacyHNSW   = "legacy_hnsw"
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

// Ping reports whether the embedding sidecar answers its health endpoint.
// It is intentionally not part of the Embedder interface: callers type-assert
// for it, so test doubles that only embed stay valid.
func (e *HTTPEmbedder) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.url+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("embed healthz: status %d", resp.StatusCode)
	}
	return nil
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

// SearchQuery defines structured filters for semantic and time-scoped photo search.
type SearchQuery struct {
	Query      string
	Role       string
	PublicOnly bool
	AlbumIDs   []int64
	From       *time.Time // Inclusive lower bound on taken_at.
	To         *time.Time // Exclusive upper bound on taken_at.
	ExcludeIDs []int64
	Limit      int
	MinScore   float64
	Diversify  bool
}

// SearchMeta provides diagnostic and scope-coverage metadata alongside search results.
type SearchMeta struct {
	Strategy       string `json:"strategy"`
	ScopeTotal     int64  `json:"scope_total"`
	VisibleDateMin string `json:"visible_date_min,omitempty"`
	VisibleDateMax string `json:"visible_date_max,omitempty"`
	ElapsedMS      int64  `json:"elapsed_ms"`
}

// SearchResult wraps search items and execution metadata.
type SearchResult struct {
	Items []SearchItem `json:"items"`
	Meta  SearchMeta   `json:"meta"`
}

type SearchService struct {
	DB             *gorm.DB
	Embedder       Embedder
	MaxText        int
	ExactThreshold int64

	schemaMu       sync.Mutex
	optimizedReady bool
	schemaChecked  time.Time
}

func NewSearchService(db *gorm.DB, embedder Embedder, maxText int) *SearchService {
	return &SearchService{
		DB:             db,
		Embedder:       embedder,
		MaxText:        maxText,
		ExactThreshold: defaultExactThreshold,
	}
}

// ParseSearchDateRange parses optional from/to date strings in CST (+08).
// Supported formats: YYYY-MM, YYYY-MM-DD, YYYY-MM-DDTHH:MM:SS, and RFC3339.
// The returned range is half-open: [from, to).
func ParseSearchDateRange(fromStr, toStr string) (*time.Time, *time.Time, error) {
	fromStr = strings.TrimSpace(fromStr)
	toStr = strings.TrimSpace(toStr)

	var fromPtr, toPtr *time.Time
	if fromStr != "" {
		t, err := parseSearchBound(fromStr, false)
		if err != nil {
			return nil, nil, ErrInvalidDateRange
		}
		fromPtr = &t
	}
	if toStr != "" {
		t, err := parseSearchBound(toStr, true)
		if err != nil {
			return nil, nil, ErrInvalidDateRange
		}
		toPtr = &t
	}
	if fromPtr != nil && toPtr != nil && !fromPtr.Before(*toPtr) {
		return nil, nil, ErrInvalidDateRange
	}
	return fromPtr, toPtr, nil
}

func parseSearchBound(raw string, isUpper bool) (time.Time, error) {
	if len(raw) == 7 {
		t, err := time.ParseInLocation("2006-01", raw, cstZone)
		if err != nil {
			return time.Time{}, err
		}
		if isUpper {
			t = t.AddDate(0, 1, 0)
		}
		return t, nil
	}
	if len(raw) == 10 {
		t, err := time.ParseInLocation("2006-01-02", raw, cstZone)
		if err != nil {
			return time.Time{}, err
		}
		if isUpper {
			t = t.AddDate(0, 0, 1)
		}
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		if isUpper {
			t = t.Add(time.Second)
		}
		return t, nil
	}
	normalized := strings.Replace(raw, " ", "T", 1)
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", normalized, cstZone); err == nil {
		if isUpper {
			t = t.Add(time.Second)
		}
		return t, nil
	}
	return time.Time{}, ErrInvalidDateRange
}

func normalizeIDList(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int64]bool, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func clampSearchLimit(limit int) int {
	if limit <= 0 {
		return defaultSearchLimit
	}
	if limit > maxSearchLimit {
		return maxSearchLimit
	}
	return limit
}

func computeOverfetch(limit int, diversify bool) int {
	if !diversify {
		return limit
	}
	n := limit * 3
	if n < limit+20 {
		n = limit + 20
	}
	if n > maxSearchLimit {
		n = maxSearchLimit
	}
	return n
}

func computeHNSWEfSearch(overfetch int) int {
	ef := overfetch * 2
	if ef < hnswMinEfSearch {
		ef = hnswMinEfSearch
	}
	if ef > hnswMaxEfSearch {
		ef = hnswMaxEfSearch
	}
	return ef
}

func (s *SearchService) exactThreshold() int64 {
	if s.ExactThreshold > 0 {
		return s.ExactThreshold
	}
	return defaultExactThreshold
}

// quietSearchDB silences GORM logging so 1024-dim vector literals are never
// written into slow SQL logs.
func quietSearchDB(db *gorm.DB, ctx context.Context) *gorm.DB {
	return db.WithContext(ctx).Session(&gorm.Session{
		Logger: logger.Default.LogMode(logger.Silent),
	})
}

// hasOptimizedSchema detects whether the search-optimization-v1 migration has
// been applied and backfilled. Once detected as ready, the result is cached
// permanently; otherwise it re-checks at most once per minute so applying the
// migration takes effect without requiring a restart.
func (s *SearchService) hasOptimizedSchema(ctx context.Context) bool {
	if s.DB == nil {
		return false
	}
	s.schemaMu.Lock()
	if s.optimizedReady {
		s.schemaMu.Unlock()
		return true
	}
	if !s.schemaChecked.IsZero() && time.Since(s.schemaChecked) < schemaRecheckInterval {
		s.schemaMu.Unlock()
		return false
	}
	s.schemaMu.Unlock()

	var colCount int64
	err := quietSearchDB(s.DB, ctx).Raw(`
		SELECT COUNT(*)
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = 'photo_embeddings'
		  AND column_name IN ('embedding_hv', 'is_public', 'taken_at')
	`).Scan(&colCount).Error
	ready := false
	if err == nil && colCount == 3 {
		var missingHV bool
		err2 := quietSearchDB(s.DB, ctx).Raw(`
			SELECT EXISTS (
				SELECT 1 FROM public.photo_embeddings
				WHERE embedding IS NOT NULL AND embedding_hv IS NULL
				LIMIT 1
			)
		`).Scan(&missingHV).Error
		ready = err2 == nil && !missingHV
	}

	s.schemaMu.Lock()
	s.schemaChecked = time.Now()
	if ready {
		s.optimizedReady = true
	}
	s.schemaMu.Unlock()
	return ready
}

// Search preserves the original SearchService signature for existing callers.
func (s *SearchService) Search(ctx context.Context, q, role string, limit int, albumID int64) ([]SearchItem, error) {
	req := SearchQuery{
		Query: q,
		Role:  role,
		Limit: limit,
	}
	if albumID > 0 {
		req.AlbumIDs = []int64{albumID}
	}
	res, err := s.Query(ctx, req)
	if err != nil {
		return nil, err
	}
	return res.Items, nil
}

// Query executes semantic or time-scoped photo retrieval with adaptive routing
// and guaranteed candidate recall within the requested SQL visibility scope.
func (s *SearchService) Query(ctx context.Context, req SearchQuery) (*SearchResult, error) {
	started := time.Now()

	q := strings.TrimSpace(req.Query)
	if req.From != nil && req.To != nil && !req.From.Before(*req.To) {
		return nil, ErrInvalidDateRange
	}

	albumIDs := normalizeIDList(req.AlbumIDs)
	excludeIDs := normalizeIDList(req.ExcludeIDs)
	publicOnly := req.PublicOnly || req.Role == "guest"
	limit := clampSearchLimit(req.Limit)
	hasNarrowFilter := len(albumIDs) > 0 || req.From != nil || req.To != nil || len(excludeIDs) > 0

	if q == "" && !hasNarrowFilter {
		return nil, ErrEmptyQuery
	}

	if q == "" {
		res, err := s.queryTimeSample(ctx, publicOnly, albumIDs, req.From, req.To, excludeIDs, limit)
		if err != nil {
			return nil, err
		}
		res.Meta.ElapsedMS = time.Since(started).Milliseconds()
		return res, nil
	}

	runes := []rune(q)
	if s.MaxText > 0 && len(runes) > s.MaxText {
		q = string(runes[:s.MaxText])
	}
	if s.Embedder == nil {
		return nil, ErrEmbedUnavailable
	}
	vec, err := s.Embedder.Embed(ctx, q)
	if err != nil {
		return nil, err
	}
	vecStr := formatVector(vec)
	overfetch := computeOverfetch(limit, req.Diversify)

	var res *SearchResult
	if s.hasOptimizedSchema(ctx) {
		res, err = s.queryOptimized(ctx, vecStr, publicOnly, albumIDs, req.From, req.To, excludeIDs, limit, overfetch, req.MinScore, req.Diversify, hasNarrowFilter)
	} else {
		res, err = s.queryLegacy(ctx, vecStr, publicOnly, albumIDs, req.From, req.To, excludeIDs, limit, overfetch, req.MinScore, req.Diversify, hasNarrowFilter)
	}
	if err != nil {
		return nil, err
	}
	res.Meta.ElapsedMS = time.Since(started).Milliseconds()
	return res, nil
}

type searchRow struct {
	model.Photo
	Score   float64 `gorm:"column:score"`
	VecText string  `gorm:"column:vec_text"`
}

func buildOptimizedWhere(publicOnly bool, albumIDs []int64, from, to *time.Time, excludeIDs []int64) (string, []any) {
	clauses := []string{"pe.embedding_hv IS NOT NULL"}
	var args []any
	if publicOnly {
		clauses = append(clauses, "pe.is_public = true")
	}
	if from != nil {
		clauses = append(clauses, "pe.taken_at >= ?")
		args = append(args, *from)
	}
	if to != nil {
		clauses = append(clauses, "pe.taken_at < ?")
		args = append(args, *to)
	}
	if len(albumIDs) == 1 {
		clauses = append(clauses, "pe.photo_id IN (SELECT photo_id FROM album_photos WHERE album_id = ?)")
		args = append(args, albumIDs[0])
	} else if len(albumIDs) > 1 {
		clauses = append(clauses, "pe.photo_id IN (SELECT photo_id FROM album_photos WHERE album_id IN ?)")
		args = append(args, albumIDs)
	}
	if len(excludeIDs) > 0 {
		clauses = append(clauses, "pe.photo_id NOT IN ?")
		args = append(args, excludeIDs)
	}
	return strings.Join(clauses, " AND "), args
}

func buildLegacyWhere(publicOnly bool, albumIDs []int64, from, to *time.Time, excludeIDs []int64) (string, []any) {
	clauses := []string{"TRUE"}
	var args []any
	if publicOnly {
		clauses = append(clauses, "p.id IN (SELECT ap.photo_id FROM album_photos ap JOIN albums a ON a.id = ap.album_id WHERE a.is_public = true)")
	}
	if from != nil {
		clauses = append(clauses, "p.taken_at >= ?")
		args = append(args, *from)
	}
	if to != nil {
		clauses = append(clauses, "p.taken_at < ?")
		args = append(args, *to)
	}
	if len(albumIDs) == 1 {
		clauses = append(clauses, "p.id IN (SELECT photo_id FROM album_photos WHERE album_id = ?)")
		args = append(args, albumIDs[0])
	} else if len(albumIDs) > 1 {
		clauses = append(clauses, "p.id IN (SELECT photo_id FROM album_photos WHERE album_id IN ?)")
		args = append(args, albumIDs)
	}
	if len(excludeIDs) > 0 {
		clauses = append(clauses, "p.id NOT IN ?")
		args = append(args, excludeIDs)
	}
	return strings.Join(clauses, " AND "), args
}

func (s *SearchService) queryOptimized(
	ctx context.Context,
	vecStr string,
	publicOnly bool,
	albumIDs []int64,
	from, to *time.Time,
	excludeIDs []int64,
	limit, overfetch int,
	minScore float64,
	diversify, hasNarrowFilter bool,
) (*SearchResult, error) {
	db := quietSearchDB(s.DB, ctx)
	whereSQL, whereArgs := buildOptimizedWhere(publicOnly, albumIDs, from, to, excludeIDs)

	var scopeTotal int64 = -1
	if hasNarrowFilter {
		if err := db.Raw("SELECT COUNT(*) FROM photo_embeddings pe WHERE "+whereSQL, whereArgs...).Scan(&scopeTotal).Error; err != nil {
			return nil, err
		}
		if scopeTotal == 0 {
			minD, maxD := s.visibleDateRangeOptimized(db, publicOnly)
			return &SearchResult{
				Items: []SearchItem{},
				Meta: SearchMeta{
					Strategy:       SearchStrategyEmptyScope,
					ScopeTotal:     0,
					VisibleDateMin: minD,
					VisibleDateMax: maxD,
				},
			}, nil
		}
	}

	var (
		rows     []searchRow
		strategy string
		err      error
	)

	if hasNarrowFilter && scopeTotal <= s.exactThreshold() {
		strategy = SearchStrategyExactHalfvec
		rows, err = s.execExactHalfvec(db, vecStr, whereSQL, whereArgs, overfetch, diversify)
		if err != nil {
			return nil, err
		}
	} else {
		if publicOnly {
			strategy = SearchStrategyHNSWPublicHV
		} else {
			strategy = SearchStrategyHNSWAllHV
		}
		rows, err = s.execHNSWHalfvec(db, vecStr, whereSQL, whereArgs, overfetch, diversify)
		if err != nil {
			return nil, err
		}

		// Guarantee candidate floor: if HNSW iterative scan returned fewer than
		// expected, verify scope cardinality and fall back to exact sort.
		if len(rows) < overfetch {
			if scopeTotal < 0 {
				if err := db.Raw("SELECT COUNT(*) FROM photo_embeddings pe WHERE "+whereSQL, whereArgs...).Scan(&scopeTotal).Error; err != nil {
					return nil, err
				}
			}
			want := int64(overfetch)
			if scopeTotal < want {
				want = scopeTotal
			}
			if int64(len(rows)) < want {
				strategy = SearchStrategyHNSWFallback
				rows, err = s.execExactHalfvec(db, vecStr, whereSQL, whereArgs, overfetch, diversify)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	items := finalizeSearchRows(rows, limit, minScore, diversify)
	meta := SearchMeta{
		Strategy:   strategy,
		ScopeTotal: scopeTotal,
	}
	if scopeTotal < 0 {
		meta.ScopeTotal = int64(len(items))
	}
	return &SearchResult{Items: items, Meta: meta}, nil
}

func (s *SearchService) execExactHalfvec(
	db *gorm.DB,
	vecStr, whereSQL string,
	whereArgs []any,
	overfetch int,
	includeVec bool,
) ([]searchRow, error) {
	// Evaluate the distance once via "ORDER BY 2, 1" while the "+ 0" blocks
	// global HNSW plans and keeps B-Tree filter indexes eligible.
	vecSelect := ""
	vecJoin := ""
	if includeVec {
		vecSelect = ", pe2.embedding_hv::text AS vec_text"
		vecJoin = " JOIN photo_embeddings pe2 ON pe2.photo_id = r.photo_id"
	}
	sql := `WITH ranked AS (
		SELECT pe.photo_id, ((pe.embedding_hv <=> ?::halfvec(1024)) + 0) AS dist
		FROM photo_embeddings pe
		WHERE ` + whereSQL + `
		ORDER BY 2, 1
		LIMIT ?
	)
	SELECT p.*, 1 - r.dist AS score` + vecSelect + `
	FROM ranked r
	JOIN photos p ON p.id = r.photo_id` + vecJoin + `
	ORDER BY r.dist, p.id`

	args := make([]any, 0, len(whereArgs)+2)
	args = append(args, vecStr)
	args = append(args, whereArgs...)
	args = append(args, overfetch)

	var rows []searchRow
	if err := db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *SearchService) execHNSWHalfvec(
	db *gorm.DB,
	vecStr, whereSQL string,
	whereArgs []any,
	overfetch int,
	includeVec bool,
) ([]searchRow, error) {
	efSearch := computeHNSWEfSearch(overfetch)
	vecSelect := ""
	vecJoin := ""
	if includeVec {
		vecSelect = ", pe2.embedding_hv::text AS vec_text"
		vecJoin = " JOIN photo_embeddings pe2 ON pe2.photo_id = r.photo_id"
	}
	// Avoid an outer SQL Sort node while enable_sort = off; final results
	// (at most 200 rows) are strictly ordered in Go after scanning.
	sql := `WITH ranked AS (
		SELECT pe.photo_id, (pe.embedding_hv <=> ?::halfvec(1024)) AS dist
		FROM photo_embeddings pe
		WHERE ` + whereSQL + `
		ORDER BY pe.embedding_hv <=> ?::halfvec(1024)
		LIMIT ?
	)
	SELECT p.*, 1 - r.dist AS score` + vecSelect + `
	FROM ranked r
	JOIN photos p ON p.id = r.photo_id` + vecJoin

	args := make([]any, 0, len(whereArgs)+3)
	args = append(args, vecStr)
	args = append(args, whereArgs...)
	args = append(args, vecStr, overfetch)

	var rows []searchRow
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL enable_sort = off").Error; err != nil {
			return err
		}
		if err := tx.Exec("SET LOCAL hnsw.iterative_scan = relaxed_order").Error; err != nil {
			return err
		}
		// SET does not accept bind parameters; these values are bounded integers.
		if err := tx.Exec(fmt.Sprintf("SET LOCAL hnsw.ef_search = %d", efSearch)).Error; err != nil {
			return err
		}
		if err := tx.Exec(fmt.Sprintf("SET LOCAL hnsw.max_scan_tuples = %d", hnswDefaultMaxScanTuple)).Error; err != nil {
			return err
		}
		return tx.Raw(sql, args...).Scan(&rows).Error
	})
	if err != nil {
		return nil, err
	}
	sortSearchRows(rows)
	return rows, nil
}

func (s *SearchService) queryLegacy(
	ctx context.Context,
	vecStr string,
	publicOnly bool,
	albumIDs []int64,
	from, to *time.Time,
	excludeIDs []int64,
	limit, overfetch int,
	minScore float64,
	diversify, hasNarrowFilter bool,
) (*SearchResult, error) {
	db := quietSearchDB(s.DB, ctx)
	whereSQL, whereArgs := buildLegacyWhere(publicOnly, albumIDs, from, to, excludeIDs)
	filtered := publicOnly || hasNarrowFilter

	var scopeTotal int64 = -1
	if hasNarrowFilter {
		if err := db.Raw(`SELECT COUNT(*) FROM photo_embeddings pe JOIN photos p ON p.id = pe.photo_id WHERE `+whereSQL, whereArgs...).Scan(&scopeTotal).Error; err != nil {
			return nil, err
		}
		if scopeTotal == 0 {
			minD, maxD := s.visibleDateRangeLegacy(db, publicOnly)
			return &SearchResult{
				Items: []SearchItem{},
				Meta: SearchMeta{
					Strategy:       SearchStrategyEmptyScope,
					ScopeTotal:     0,
					VisibleDateMin: minD,
					VisibleDateMax: maxD,
				},
			}, nil
		}
	}

	vecSelect := ""
	vecJoin := ""
	if diversify {
		vecSelect = ", pe2.embedding::text AS vec_text"
		vecJoin = " JOIN photo_embeddings pe2 ON pe2.photo_id = r.photo_id"
	}

	var (
		rows     []searchRow
		strategy string
		err      error
	)
	if filtered {
		strategy = SearchStrategyLegacyExact
		// Single distance evaluation per row via "ORDER BY 2, 1" inside CTE.
		sql := `WITH ranked AS (
			SELECT pe.photo_id, ((pe.embedding <=> ?::vector) + 0) AS dist
			FROM photo_embeddings pe
			JOIN photos p ON p.id = pe.photo_id
			WHERE ` + whereSQL + `
			ORDER BY 2, 1
			LIMIT ?
		)
		SELECT p.*, 1 - r.dist AS score` + vecSelect + `
		FROM ranked r
		JOIN photos p ON p.id = r.photo_id` + vecJoin + `
		ORDER BY r.dist, p.id`

		args := make([]any, 0, len(whereArgs)+2)
		args = append(args, vecStr)
		args = append(args, whereArgs...)
		args = append(args, overfetch)
		err = db.Raw(sql, args...).Scan(&rows).Error
	} else {
		strategy = SearchStrategyLegacyHNSW
		efSearch := computeHNSWEfSearch(overfetch)
		sql := `WITH ranked AS (
			SELECT pe.photo_id, (pe.embedding <=> ?::vector) AS dist
			FROM photo_embeddings pe
			ORDER BY pe.embedding <=> ?::vector
			LIMIT ?
		)
		SELECT p.*, 1 - r.dist AS score` + vecSelect + `
		FROM ranked r
		JOIN photos p ON p.id = r.photo_id` + vecJoin
		err = db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("SET LOCAL enable_sort = off").Error; err != nil {
				return err
			}
			if err := tx.Exec(fmt.Sprintf("SET LOCAL hnsw.ef_search = %d", efSearch)).Error; err != nil {
				return err
			}
			return tx.Raw(sql, vecStr, vecStr, overfetch).Scan(&rows).Error
		})
		if err == nil {
			sortSearchRows(rows)
		}
	}
	if err != nil {
		return nil, err
	}

	items := finalizeSearchRows(rows, limit, minScore, diversify)
	meta := SearchMeta{
		Strategy:   strategy,
		ScopeTotal: scopeTotal,
	}
	if scopeTotal < 0 {
		meta.ScopeTotal = int64(len(items))
	}
	return &SearchResult{Items: items, Meta: meta}, nil
}

// queryTimeSample returns evenly-spaced photos across a filtered scope when no
// text query is provided (e.g., pure date-range or album retrieval).
func (s *SearchService) queryTimeSample(
	ctx context.Context,
	publicOnly bool,
	albumIDs []int64,
	from, to *time.Time,
	excludeIDs []int64,
	limit int,
) (*SearchResult, error) {
	db := quietSearchDB(s.DB, ctx)
	whereSQL, whereArgs := buildLegacyWhere(publicOnly, albumIDs, from, to, excludeIDs)

	var scopeTotal int64
	if err := db.Raw("SELECT COUNT(*) FROM photos p WHERE "+whereSQL, whereArgs...).Scan(&scopeTotal).Error; err != nil {
		return nil, err
	}
	if scopeTotal == 0 {
		minD, maxD := s.visibleDateRangeLegacy(db, publicOnly)
		return &SearchResult{
			Items: []SearchItem{},
			Meta: SearchMeta{
				Strategy:       SearchStrategyEmptyScope,
				ScopeTotal:     0,
				VisibleDateMin: minD,
				VisibleDateMax: maxD,
			},
		}, nil
	}

	sql := `WITH ordered AS (
		SELECT p.id,
		       row_number() OVER (ORDER BY p.taken_at DESC NULLS LAST, p.id DESC) AS pos,
		       count(*) OVER () AS total
		FROM photos p
		WHERE ` + whereSQL + `
	)
	SELECT p.*, 0::double precision AS score
	FROM ordered o
	JOIN photos p ON p.id = o.id
	WHERE mod(o.pos - 1, GREATEST(1, ceil(o.total::numeric / ?)::bigint)) = 0
	ORDER BY p.taken_at DESC NULLS LAST, p.id DESC
	LIMIT ?`

	args := make([]any, 0, len(whereArgs)+2)
	args = append(args, whereArgs...)
	args = append(args, limit, limit)

	var items []SearchItem
	if err := db.Raw(sql, args...).Scan(&items).Error; err != nil {
		return nil, err
	}
	if items == nil {
		items = []SearchItem{}
	}
	return &SearchResult{
		Items: items,
		Meta: SearchMeta{
			Strategy:   SearchStrategyTimeSample,
			ScopeTotal: scopeTotal,
		},
	}, nil
}

func (s *SearchService) visibleDateRangeOptimized(db *gorm.DB, publicOnly bool) (string, string) {
	var row struct {
		MinDate *string `gorm:"column:min_date"`
		MaxDate *string `gorm:"column:max_date"`
	}
	q := `SELECT to_char(min(taken_at) AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD') AS min_date,
	             to_char(max(taken_at) AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD') AS max_date
	      FROM photo_embeddings
	      WHERE taken_at IS NOT NULL`
	if publicOnly {
		q += " AND is_public = true"
	}
	if err := db.Raw(q).Scan(&row).Error; err != nil {
		return "", ""
	}
	minD, maxD := "", ""
	if row.MinDate != nil {
		minD = *row.MinDate
	}
	if row.MaxDate != nil {
		maxD = *row.MaxDate
	}
	return minD, maxD
}

func (s *SearchService) visibleDateRangeLegacy(db *gorm.DB, publicOnly bool) (string, string) {
	var row struct {
		MinDate *string `gorm:"column:min_date"`
		MaxDate *string `gorm:"column:max_date"`
	}
	q := `SELECT to_char(min(p.taken_at) AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD') AS min_date,
	             to_char(max(p.taken_at) AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD') AS max_date
	      FROM photos p
	      WHERE p.taken_at IS NOT NULL`
	if publicOnly {
		q += " AND p.id IN (SELECT ap.photo_id FROM album_photos ap JOIN albums a ON a.id = ap.album_id WHERE a.is_public = true)"
	}
	if err := db.Raw(q).Scan(&row).Error; err != nil {
		return "", ""
	}
	minD, maxD := "", ""
	if row.MinDate != nil {
		minD = *row.MinDate
	}
	if row.MaxDate != nil {
		maxD = *row.MaxDate
	}
	return minD, maxD
}

func sortSearchRows(rows []searchRow) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Score != rows[j].Score {
			return rows[i].Score > rows[j].Score
		}
		return rows[i].ID < rows[j].ID
	})
}

func finalizeSearchRows(rows []searchRow, limit int, minScore float64, diversify bool) []SearchItem {
	if len(rows) == 0 {
		return []SearchItem{}
	}
	filtered := rows
	if minScore > 0 {
		filtered = make([]searchRow, 0, len(rows))
		for _, r := range rows {
			if r.Score >= minScore {
				filtered = append(filtered, r)
			}
		}
	}
	if len(filtered) == 0 {
		return []SearchItem{}
	}
	if diversify {
		cands := make([]scoredCandidate, len(filtered))
		for i, r := range filtered {
			cands[i] = scoredCandidate{
				item:   SearchItem{Photo: r.Photo, Score: r.Score},
				vector: parseVectorLiteral(r.VecText),
			}
		}
		return diversifyCandidates(cands, limit, defaultBurstWindow)
	}
	n := len(filtered)
	if n > limit {
		n = limit
	}
	out := make([]SearchItem, n)
	for i := 0; i < n; i++ {
		out[i] = SearchItem{Photo: filtered[i].Photo, Score: filtered[i].Score}
	}
	return out
}
