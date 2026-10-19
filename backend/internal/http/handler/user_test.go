package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"mckit/backend/internal/auth"
	"mckit/backend/internal/http/middleware"
	"mckit/backend/internal/service/user"
)

// newUserTestRouter 按 main.go 的接线方式搭建用户路由，用于鉴权 / CSRF 行为测试。
func newUserTestRouter(t *testing.T) (chi.Router, *user.Service, auth.SessionStore) {
	t.Helper()
	repo := user.NewMemoryRepo()
	sessions := auth.NewMemorySessionStore()
	svc := user.NewService(repo, sessions, 5, 900)
	h := NewUserHandler(svc, false, "mc_user_session")

	r := chi.NewRouter()
	r.Route("/user", func(ur chi.Router) {
		ur.Post("/register", h.Register)
		ur.Post("/login", h.Login)
		ur.Group(func(authed chi.Router) {
			authed.Use(middleware.RequireUserAuth(svc))
			authed.Post("/logout", h.Logout)
			authed.Post("/me", h.Me)
			authed.Post("/favorites/list", h.ListFavorites)
			authed.Group(func(csrf chi.Router) {
				csrf.Use(middleware.UserCSRF)
				csrf.Post("/favorites/toggle", h.ToggleFavorite)
			})
		})
	})
	return r, svc, sessions
}

// doJSON 发送 JSON 请求；cookie/csrf 为空则不附加。
func doJSON(r chi.Router, method, path, cookie, csrf string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "mc_user_session", Value: cookie})
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestUserRoutesRequireLogin(t *testing.T) {
	r, _, _ := newUserTestRouter(t)

	for _, path := range []string{"/user/me", "/user/favorites/list", "/user/favorites/toggle"} {
		rec := doJSON(r, http.MethodPost, path, "", "", map[string]string{"kind": "tool", "object_id": "x"})
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s 未登录应返回 401，实际 %d", path, rec.Code)
		}
	}
}

func TestUserWritesRequireCSRF(t *testing.T) {
	r, svc, _ := newUserTestRouter(t)
	ctx := context.Background()
	u, err := svc.Register(ctx, "tester", "password123")
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Login(ctx, "tester", "password123", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	// 有会话但缺 CSRF → 403
	rec := doJSON(r, http.MethodPost, "/user/favorites/toggle", res.SessionID, "",
		map[string]string{"kind": "tool", "object_id": "recipes"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("缺 CSRF 应返回 403，实际 %d：%s", rec.Code, rec.Body.String())
	}

	// 会话 + CSRF → 200 且已收藏
	rec = doJSON(r, http.MethodPost, "/user/favorites/toggle", res.SessionID, res.CSRFToken,
		map[string]string{"kind": "tool", "object_id": "recipes", "label": "配方", "url": "/recipes"})
	if rec.Code != http.StatusOK {
		t.Fatalf("带 CSRF 应返回 200，实际 %d：%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Favorited bool `json:"favorited"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Data.Favorited {
		t.Fatal("首次收藏应返回 favorited=true")
	}

	// 读接口仅需会话
	rec = doJSON(r, http.MethodPost, "/user/favorites/list", res.SessionID, "", map[string]string{})
	if rec.Code != http.StatusOK {
		t.Fatalf("读接口应返回 200，实际 %d", rec.Code)
	}
	_ = u
}

func TestUserRoutesRejectAdminSession(t *testing.T) {
	r, _, sessions := newUserTestRouter(t)

	// 伪造一个管理员角色的会话塞入同一存储，并携带其 Cookie 访问用户接口。
	if err := sessions.Create(context.Background(), auth.Session{
		SessionID: "admin-sid",
		AdminID:   "admin-1",
		Username:  "root",
		Role:      "admin",
		CSRFToken: "admin-csrf",
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	rec := doJSON(r, http.MethodPost, "/user/me", "admin-sid", "admin-csrf", map[string]string{})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("管理会话不应通过用户鉴权，实际 %d", rec.Code)
	}
}
