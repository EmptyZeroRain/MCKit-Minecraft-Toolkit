package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"mckit/backend/internal/http/middleware"
	"mckit/backend/internal/http/response"
)

// newTestRouter 构造一个不依赖数据库的测试路由（阶段 A 仅含健康检查）。
func newTestRouter(_ *gorm.DB) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recover)
	api := chi.NewRouter()
	registerRoutes(api, nil, nil)
	api.NotFound(func(w http.ResponseWriter, r *http.Request) {
		response.NotFound(w, r)
	})
	api.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		response.Error(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "请求方法不允许", nil)
	})
	r.Mount("/api/v1", api)
	return r
}

func TestHealthCheck(t *testing.T) {
	router := newTestRouter(nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", rec.Code)
	}
	var body struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是合法 JSON: %v", err)
	}
	if body.Data["status"] != "ok" {
		t.Fatalf("健康检查 status 异常: %v", body.Data)
	}
}

func TestHealthCheckGetMethodNotAllowed(t *testing.T) {
	router := newTestRouter(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET 访问应返回 405，实际 %d", rec.Code)
	}
}

func TestNotFoundReturnsJSON(t *testing.T) {
	router := newTestRouter(nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/does-not-exist", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("期望 404，实际 %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("404 应返回 JSON，实际 Content-Type=%q", ct)
	}
	if !strings.Contains(rec.Body.String(), `"request_id"`) {
		t.Fatalf("错误响应应包含 request_id：%s", rec.Body.String())
	}
	// 错误响应不得泄露内部信息（堆栈/路径/SQL）
	for _, leak := range []string{"main.go", "panic", "sql:", "stack"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Fatalf("错误响应泄漏内部信息 %q：%s", leak, rec.Body.String())
		}
	}
}

func TestRequestIDHeader(t *testing.T) {
	router := newTestRouter(nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Header().Get("X-Request-Id") == "" {
		t.Fatal("响应应包含 X-Request-Id")
	}
}
