package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"mckit/backend/internal/auth"
)

// fakeSessionSvc 按 sessionID 返回会话，供 OptionalAuth 测试使用。
type fakeSessionSvc struct {
	sessions map[string]*auth.Session
	err      error
}

func (f fakeSessionSvc) GetSession(_ context.Context, id string) (*auth.Session, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.sessions[id], nil
}

// TestOptionalAuth 覆盖 mcsrc 自动准备的鉴权语义：
// OptionalAuth 只负责「尽力识别登录者」，**绝不**返回 401，
// 未登录 / 会话失效 / 会话服务异常一律按匿名继续（额度限制由业务层负责）。
func TestOptionalAuth(t *testing.T) {
	svc := fakeSessionSvc{sessions: map[string]*auth.Session{
		"s1": {SessionID: "s1", AccountID: "a1", Role: "admin"},
	}}

	cases := []struct {
		name     string
		cookie   string
		wantRole string // 空字符串表示期望匿名
	}{
		{"无 Cookie 按匿名放行", "", ""},
		{"有效会话注入 context", "s1", "admin"},
		{"未知会话 ID 按匿名放行", "ghost", ""},
		{"空 Cookie 值按匿名放行", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got *auth.Session
			h := OptionalAuth(svc)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = SessionFrom(r.Context())
			}))
			req := httptest.NewRequest(http.MethodPost, "/api/v1/mcsrc/prepare", nil)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: cookieName, Value: tc.cookie})
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("OptionalAuth 不应拒绝请求，实际 %d", rec.Code)
			}
			if tc.wantRole == "" {
				if got != nil {
					t.Fatalf("期望匿名，实际注入会话 %+v", got)
				}
				return
			}
			if got == nil || got.Role != tc.wantRole {
				t.Fatalf("期望注入角色 %q，实际 %+v", tc.wantRole, got)
			}
		})
	}
}

// TestOptionalAuthNeverFails 覆盖会话服务异常：仍按匿名放行，不返回 401/500。
func TestOptionalAuthNeverFails(t *testing.T) {
	svc := fakeSessionSvc{err: errors.New("会话存储不可用")}
	called := false
	h := OptionalAuth(svc)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcsrc/prepare", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: "s1"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !called || rec.Code != http.StatusOK {
		t.Fatalf("会话服务异常时应按匿名放行，实际 called=%v status=%d", called, rec.Code)
	}
}

// TestOptionalAuthNilService 覆盖未注入认证服务（svc 为 nil）：不应 panic。
func TestOptionalAuthNilService(t *testing.T) {
	called := false
	h := OptionalAuth(nil)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcsrc/prepare", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !called || rec.Code != http.StatusOK {
		t.Fatalf("未注入服务时应直接放行，实际 called=%v status=%d", called, rec.Code)
	}
}
