package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"shutterseek/internal/service"
)

type unitStubEmbedder struct {
	vec []float32
	err error
}

func (s *unitStubEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return s.vec, s.err
}

func TestSearchHandler_Validation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{
		SearchSvc: service.NewSearchService(nil, &unitStubEmbedder{err: service.ErrEmbedUnavailable}, 200),
	}

	tests := []struct {
		name     string
		url      string
		wantCode int
	}{
		{"missing_query_and_dates", "/api/v1/search?q=", http.StatusBadRequest},
		{"bad_from_date", "/api/v1/search?q=sea&from=2020-99-99", http.StatusBadRequest},
		{"reversed_dates", "/api/v1/search?q=sea&from=2020-09-01&to=2020-08-01", http.StatusBadRequest},
		{"embed_unavailable", "/api/v1/search?q=sea&from=2020-06-01&to=2020-08-31", http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, tt.url, nil)
			c.Set("role", "admin")
			h.Search(c)
			if w.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d; body = %s", w.Code, tt.wantCode, w.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("non-JSON response: %s", w.Body.String())
			}
			if _, ok := body["error"]; !ok {
				t.Fatalf("expected error key in %v", body)
			}
		})
	}
}
