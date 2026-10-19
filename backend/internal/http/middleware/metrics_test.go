package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/service/metrics"
)

// recordingStore 仅记录 API 聚合行的路由模板，供断言。
type recordingStore struct {
	routes []string
}

func (s *recordingStore) UpsertAPIStats(_ context.Context, rows []domain.APIStatDaily) error {
	for _, r := range rows {
		s.routes = append(s.routes, r.Route)
	}
	return nil
}

func (s *recordingStore) UpsertToolStats(context.Context, []domain.ToolStatDaily) error { return nil }
func (s *recordingStore) UpsertResourceStats(context.Context, []domain.ResourceStatDaily) error {
	return nil
}
func (s *recordingStore) UpsertSearchStats(context.Context, []domain.SearchStatDaily) error {
	return nil
}
func (s *recordingStore) PruneBefore(context.Context, time.Time) (int64, error) { return 0, nil }

func TestMetricsCapturesMountedRoutePattern(t *testing.T) {
	store := &recordingStore{}
	c := metrics.New(store, metrics.Options{FlushInterval: time.Hour})

	root := chi.NewRouter()
	root.Use(Metrics(c))
	api := chi.NewRouter()
	api.Post("/tools/{slug}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	root.Mount("/api/v1", api)

	root.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodPost, "/api/v1/tools/region-inspect", nil))

	if err := c.Flush(context.Background()); err != nil {
		t.Fatalf("Flush 失败: %v", err)
	}
	if len(store.routes) != 1 {
		t.Fatalf("期望 1 条 API 指标，实际 %d", len(store.routes))
	}
	if store.routes[0] != "/api/v1/tools/{slug}" {
		t.Fatalf("路由模板 = %q，期望 /api/v1/tools/{slug}（不得包含具体 slug 值）", store.routes[0])
	}
}

func TestMetricsUnmatchedRoute(t *testing.T) {
	store := &recordingStore{}
	c := metrics.New(store, metrics.Options{FlushInterval: time.Hour})

	root := chi.NewRouter()
	root.Use(Metrics(c))
	// 必须先注册至少一条路由，chi 才会构建含中间件的 handler 链。
	root.Post("/known", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	root.NotFound(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })

	root.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/nope", nil))
	if err := c.Flush(context.Background()); err != nil {
		t.Fatalf("Flush 失败: %v", err)
	}
	if len(store.routes) != 1 || store.routes[0] != metrics.UnmatchedRoute {
		t.Fatalf("未匹配路由应归并为 %q，实际 %+v", metrics.UnmatchedRoute, store.routes)
	}
}

func TestMetricsNilCollectorIsNoop(t *testing.T) {
	h := Metrics(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", rec.Code)
	}
}
