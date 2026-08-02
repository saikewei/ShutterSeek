package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
	vec   []float32
	err   error
	calls int
}

func (s *stubEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	s.calls++
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
