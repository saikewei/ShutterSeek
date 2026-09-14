package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// embedOnlyEmbedder implements Embedder but deliberately not the optional
// pinger, mirroring the search service's test doubles.
type embedOnlyEmbedder struct{}

func (embedOnlyEmbedder) Embed(context.Context, string) ([]float32, error) {
	return nil, nil
}

type failingPinger struct{ embedOnlyEmbedder }

func (failingPinger) Ping(context.Context) error { return errors.New("down") }

type workingPinger struct{ embedOnlyEmbedder }

func (workingPinger) Ping(context.Context) error { return nil }

func TestStatsSnapshotWithoutDependencies(t *testing.T) {
	svc := NewStatsService(nil, nil, nil, nil)
	got := svc.Snapshot(context.Background())

	if got.DBOK || got.RedisOK || got.EmbedOK {
		t.Fatalf("health flags must be false with no dependencies: %+v", got)
	}
	if got.Photos != 0 || got.Users != 0 || got.Logs != 0 {
		t.Fatalf("counters must stay zero with no DB: %+v", got)
	}
	if got.UptimeSeconds < 0 {
		t.Fatalf("uptime must never be negative: %d", got.UptimeSeconds)
	}
	if got.StartedAt.IsZero() {
		t.Fatal("started_at must be recorded at construction time")
	}
}

func TestStatsSnapshotEmbedderWithoutPing(t *testing.T) {
	svc := NewStatsService(nil, nil, nil, embedOnlyEmbedder{})
	if svc.Snapshot(context.Background()).EmbedOK {
		t.Fatal("an embedder that cannot be pinged must not report as healthy")
	}
}

func TestStatsSnapshotEmbedderPingResult(t *testing.T) {
	ok := NewStatsService(nil, nil, nil, workingPinger{})
	if !ok.Snapshot(context.Background()).EmbedOK {
		t.Fatal("a successful ping must report as healthy")
	}

	bad := NewStatsService(nil, nil, nil, failingPinger{})
	if bad.Snapshot(context.Background()).EmbedOK {
		t.Fatal("a failed ping must report as unhealthy")
	}
}

func TestHTTPEmbedderPing(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{"ok", http.StatusOK, false},
		{"unavailable", http.StatusServiceUnavailable, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			err := NewHTTPEmbedder(srv.URL, time.Second, "").Ping(context.Background())
			if tc.wantErr && err == nil {
				t.Fatalf("status %d must report an error", tc.status)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("status %d must not report an error: %v", tc.status, err)
			}
			if gotPath != "/healthz" {
				t.Fatalf("expected the health endpoint, got %q", gotPath)
			}
		})
	}
}
