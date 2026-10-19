package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mckit/backend/internal/auth"
	"mckit/backend/internal/domain"
)

// TestRequireRole 覆盖需求 §6.2 / §13.1：角色权限判定。
func TestRequireRole(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cases := []struct {
		name        string
		withSession bool
		role        string
		allowed     []string
		wantStatus  int
	}{
		{"无会话返回 401", false, "", []string{domain.RoleAdmin}, http.StatusUnauthorized},
		{"editor 访问 admin 专属 403", true, domain.RoleEditor, []string{domain.RoleAdmin}, http.StatusForbidden},
		{"admin 通过", true, domain.RoleAdmin, []string{domain.RoleAdmin}, http.StatusOK},
		{"editor 命中允许集合", true, domain.RoleEditor, []string{domain.RoleAdmin, domain.RoleEditor}, http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := RequireRole(tc.allowed...)(next)
			req := httptest.NewRequest(http.MethodPost, "/admin/x", nil)
			if tc.withSession {
				req = req.WithContext(WithSession(req.Context(), &auth.Session{
					AdminID: "a", Role: tc.role,
				}))
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("期望 %d，实际 %d（body=%s）", tc.wantStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

// TestCSRF 覆盖需求 §6.1 / §11.1：Cookie 会话写操作 CSRF 双重提交校验。
func TestCSRF(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// 无会话 → 401
	req := httptest.NewRequest(http.MethodPost, "/admin/x", nil)
	rec := httptest.NewRecorder()
	CSRF(next).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("无会话期望 401，实际 %d", rec.Code)
	}

	// 有会话但缺少 token → 403
	req = httptest.NewRequest(http.MethodPost, "/admin/x", nil)
	req = req.WithContext(WithSession(req.Context(), &auth.Session{CSRFToken: "tok"}))
	rec = httptest.NewRecorder()
	CSRF(next).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("缺 token 期望 403，实际 %d", rec.Code)
	}

	// token 不匹配 → 403
	req = httptest.NewRequest(http.MethodPost, "/admin/x", nil)
	req.Header.Set("X-CSRF-Token", "wrong")
	req = req.WithContext(WithSession(req.Context(), &auth.Session{CSRFToken: "tok"}))
	rec = httptest.NewRecorder()
	CSRF(next).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("token 不匹配期望 403，实际 %d", rec.Code)
	}

	// token 匹配 → 200
	req = httptest.NewRequest(http.MethodPost, "/admin/x", nil)
	req.Header.Set("X-CSRF-Token", "tok")
	req = req.WithContext(WithSession(req.Context(), &auth.Session{CSRFToken: "tok"}))
	rec = httptest.NewRecorder()
	CSRF(next).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("token 匹配期望 200，实际 %d", rec.Code)
	}

	// GET 直接放行（无会话也不拦）
	req = httptest.NewRequest(http.MethodGet, "/admin/x", nil)
	rec = httptest.NewRecorder()
	CSRF(next).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET 期望 200，实际 %d", rec.Code)
	}
}

// TestCORS 覆盖需求 §8.6：仅允许配置来源，禁止通配符 + 凭证。
func TestCORS(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := CORS([]string{"https://ok.example"})(next)

	// 允许来源
	req := httptest.NewRequest(http.MethodPost, "/api/v1/health", nil)
	req.Header.Set("Origin", "https://ok.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://ok.example" {
		t.Fatalf("允许来源应回填 ACAO，实际 %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("允许来源应携带 Allow-Credentials")
	}

	// 未允许来源不回填
	req = httptest.NewRequest(http.MethodPost, "/api/v1/health", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("未允许来源不应回填 ACAO，实际 %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}

	// OPTIONS 预检直接 204
	req = httptest.NewRequest(http.MethodOptions, "/api/v1/health", nil)
	req.Header.Set("Origin", "https://ok.example")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("预检期望 204，实际 %d", rec.Code)
	}
}

// TestSecurityHeaders 覆盖需求 §11.1：安全响应头存在。
func TestSecurityHeaders(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	rec := httptest.NewRecorder()
	SecurityHeaders(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	for _, h := range []string{"X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy", "Content-Security-Policy"} {
		if rec.Header().Get(h) == "" {
			t.Errorf("缺少安全响应头 %s", h)
		}
	}
}

// TestRecoverHidesPanic 覆盖需求 §8.6 / §13.2：panic 不泄露内部信息。
func TestRecoverHidesPanic(t *testing.T) {
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("secret stack trace")
	})
	rec := httptest.NewRecorder()
	Recover(next).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/health", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("期望 500，实际 %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret stack trace") {
		t.Fatalf("响应不得泄露 panic 详情：%s", rec.Body.String())
	}
}

// TestRequestIDPassthrough 请求 ID 透传与生成。
func TestRequestIDPassthrough(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RequestIDFromCtx(r.Context()) == "" {
			t.Error("context 中应有请求 ID")
		}
		w.WriteHeader(http.StatusOK)
	})

	// 透传客户端 ID
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Request-ID", "client-id-123")
	rec := httptest.NewRecorder()
	RequestID(next).ServeHTTP(rec, req)
	if rec.Header().Get("X-Request-ID") != "client-id-123" {
		t.Fatalf("应透传客户端请求 ID，实际 %q", rec.Header().Get("X-Request-ID"))
	}

	// 缺失时自动生成
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	rec = httptest.NewRecorder()
	RequestID(next).ServeHTTP(rec, req)
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("缺失时应生成请求 ID")
	}
}

// TestClientIP 去除端口。
func TestClientIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "203.0.113.7:54321"
	if got := ClientIP(req); got != "203.0.113.7" {
		t.Fatalf("ClientIP=%q，期望 203.0.113.7", got)
	}
}
