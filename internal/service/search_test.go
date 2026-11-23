package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"shutterseek/internal/model"
)

func vec1024() []float32 {
	v := make([]float32, 1024)
	for i := range v {
		v[i] = 0.03125
	}
	return v
}

func embedJSON(t *testing.T, w http.ResponseWriter, vec []float32) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	parts := make([]string, len(vec))
	for i, f := range vec {
		parts[i] = fmt.Sprintf("%g", f)
	}
	fmt.Fprintf(w, `{"dim":1024,"vector":[%s],"model":"bge-m3-int8"}`, strings.Join(parts, ","))
}

func TestHTTPEmbedderSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embed" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["text"] != "海边" {
			t.Errorf("text = %q", body["text"])
		}
		embedJSON(t, w, vec1024())
	}))
	defer srv.Close()

	e := NewHTTPEmbedder(srv.URL, time.Second, "")
	vec, err := e.Embed(context.Background(), "海边")
	if err != nil {
		t.Fatal(err)
	}
	if len(vec) != 1024 {
		t.Fatalf("len = %d", len(vec))
	}
}

func TestHTTPEmbedderSendsToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok123" {
			t.Errorf("Authorization = %q", got)
		}
		embedJSON(t, w, vec1024())
	}))
	defer srv.Close()

	e := NewHTTPEmbedder(srv.URL, time.Second, "tok123")
	if _, err := e.Embed(context.Background(), "海边"); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPEmbedderTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		embedJSON(t, w, vec1024())
	}))
	defer srv.Close()

	e := NewHTTPEmbedder(srv.URL, 50*time.Millisecond, "")
	_, err := e.Embed(context.Background(), "海边")
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("err = %v", err)
	}
}

func TestHTTPEmbedderBadDim(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"dim":512,"vector":[],"model":"x"}`)
	}))
	defer srv.Close()

	e := NewHTTPEmbedder(srv.URL, time.Second, "")
	_, err := e.Embed(context.Background(), "海边")
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("err = %v", err)
	}
}

func TestFormatVector(t *testing.T) {
	got := formatVector([]float32{0.1, -0.2})
	if got != "[0.1,-0.2]" {
		t.Fatalf("got %q", got)
	}
}

type stubEmbedder struct {
	vec      []float32
	err      error
	calls    int
	lastText string
}

func (s *stubEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	s.calls++
	s.lastText = text
	return s.vec, s.err
}

func TestCachedEmbedderNilRedisPassthrough(t *testing.T) {
	inner := &stubEmbedder{vec: vec1024()}
	c := NewCachedEmbedder(inner, nil)
	v, err := c.Embed(context.Background(), "海边")
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 1024 || inner.calls != 1 {
		t.Fatalf("v=%d calls=%d", len(v), inner.calls)
	}
}

func TestParseSearchDateRange(t *testing.T) {
	from, to, err := ParseSearchDateRange("2020-06-01", "2020-08-31")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	wantFrom := time.Date(2020, 6, 1, 0, 0, 0, 0, cstZone)
	wantTo := time.Date(2020, 9, 1, 0, 0, 0, 0, cstZone)
	if !from.Equal(wantFrom) || !to.Equal(wantTo) {
		t.Fatalf("got [%v, %v), want [%v, %v)", from, to, wantFrom, wantTo)
	}

	// Single day should produce a 24h half-open range.
	from, to, err = ParseSearchDateRange("2020-06-15", "2020-06-15")
	if err != nil {
		t.Fatalf("single day err: %v", err)
	}
	if to.Sub(*from) != 24*time.Hour {
		t.Fatalf("single day span = %v, want 24h", to.Sub(*from))
	}

	// Month granularity.
	from, to, err = ParseSearchDateRange("2020-06", "2020-06")
	if err != nil {
		t.Fatalf("month err: %v", err)
	}
	if !from.Equal(time.Date(2020, 6, 1, 0, 0, 0, 0, cstZone)) || !to.Equal(time.Date(2020, 7, 1, 0, 0, 0, 0, cstZone)) {
		t.Fatalf("month range = [%v, %v)", from, to)
	}

	// Empty bounds are valid.
	from, to, err = ParseSearchDateRange("", "")
	if err != nil || from != nil || to != nil {
		t.Fatalf("empty bounds = (%v, %v, %v)", from, to, err)
	}

	// Reversed range must fail.
	if _, _, err := ParseSearchDateRange("2020-09-01", "2020-08-31"); !errors.Is(err, ErrInvalidDateRange) {
		t.Fatalf("reversed range err = %v, want ErrInvalidDateRange", err)
	}

	// Malformed date must fail.
	if _, _, err := ParseSearchDateRange("not-a-date", ""); !errors.Is(err, ErrInvalidDateRange) {
		t.Fatalf("bad date err = %v, want ErrInvalidDateRange", err)
	}
}

func TestSearchQueryValidation(t *testing.T) {
	svc := NewSearchService(nil, &stubEmbedder{err: ErrEmbedUnavailable}, 5)

	if _, err := svc.Query(context.Background(), SearchQuery{Query: "   "}); !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("empty query err = %v, want ErrEmptyQuery", err)
	}

	now := time.Now()
	earlier := now.Add(-time.Hour)
	if _, err := svc.Query(context.Background(), SearchQuery{Query: "sea", From: &now, To: &earlier}); !errors.Is(err, ErrInvalidDateRange) {
		t.Fatalf("invalid range err = %v, want ErrInvalidDateRange", err)
	}

	// MaxText truncates before calling Embedder.
	emb := &stubEmbedder{err: ErrEmbedUnavailable}
	svc = NewSearchService(nil, emb, 4)
	if _, err := svc.Query(context.Background(), SearchQuery{Query: "abcdef"}); !errors.Is(err, ErrEmbedUnavailable) {
		t.Fatalf("embed err = %v", err)
	}
	if emb.lastText != "abcd" {
		t.Fatalf("truncated query = %q, want abcd", emb.lastText)
	}
}

func TestBuildOptimizedWhere(t *testing.T) {
	from := time.Date(2020, 6, 1, 0, 0, 0, 0, cstZone)
	to := time.Date(2020, 7, 1, 0, 0, 0, 0, cstZone)
	sql, args := buildOptimizedWhere(true, []int64{503, 12}, &from, &to, []int64{99})
	for _, want := range []string{
		"pe.embedding_hv IS NOT NULL",
		"pe.is_public = true",
		"pe.taken_at >= ?",
		"pe.taken_at < ?",
		"album_id IN ?",
		"pe.photo_id NOT IN ?",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("where SQL %q missing %q", sql, want)
		}
	}
	if len(args) != 4 {
		t.Fatalf("args len = %d, want 4", len(args))
	}
}

func TestDiversifyCandidates_BurstCollapseMMRAndBackfill(t *testing.T) {
	t0 := time.Date(2020, 7, 1, 18, 0, 0, 0, cstZone)
	cands := []scoredCandidate{
		{item: SearchItem{Photo: model.Photo{ID: 1, TakenAt: t0}, Score: 0.90}, vector: []float32{1, 0, 0}},
		// Burst duplicate within 2 seconds of ID 1.
		{item: SearchItem{Photo: model.Photo{ID: 2, TakenAt: t0.Add(2 * time.Second)}, Score: 0.89}, vector: []float32{0.99, 0.1, 0}},
		// Visually near-duplicate of ID 1 but taken later.
		{item: SearchItem{Photo: model.Photo{ID: 3, TakenAt: t0.Add(time.Hour)}, Score: 0.80}, vector: []float32{0.98, 0.2, 0}},
		// Distinct photo taken later (slightly lower raw score than ID 3, orthogonal vector).
		{item: SearchItem{Photo: model.Photo{ID: 4, TakenAt: t0.Add(2 * time.Hour)}, Score: 0.78}, vector: []float32{0, 1, 0}},
	}

	// Limit 2 should pick ID 1 first, skip burst ID 2, and prefer orthogonal ID 4 over near-dup ID 3.
	got := diversifyCandidates(cands, 2, defaultBurstWindow)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ID != 1 || got[1].ID != 4 {
		t.Fatalf("ids = [%d, %d], want [1, 4]", got[0].ID, got[1].ID)
	}

	// Requesting all 4 must backfill the deferred burst shot so callers are never starved.
	gotAll := diversifyCandidates(cands, 4, defaultBurstWindow)
	if len(gotAll) != 4 {
		t.Fatalf("backfill len = %d, want 4", len(gotAll))
	}
	if gotAll[3].ID != 2 {
		t.Fatalf("expected deferred burst photo 2 last, got %d", gotAll[3].ID)
	}
}

func TestParseVectorLiteral(t *testing.T) {
	v := parseVectorLiteral("[3, 4]")
	if len(v) != 2 {
		t.Fatalf("len = %d, want 2", len(v))
	}
	if v[0] < 0.59 || v[0] > 0.61 || v[1] < 0.79 || v[1] > 0.81 {
		t.Fatalf("unexpected normalized vector: %v", v)
	}
	if parseVectorLiteral("") != nil || parseVectorLiteral("[0,0]") != nil || parseVectorLiteral("bad") != nil {
		t.Fatal("expected nil for invalid or zero vector")
	}
}
