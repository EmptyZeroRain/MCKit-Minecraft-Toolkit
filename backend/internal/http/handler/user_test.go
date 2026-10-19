package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"mckit/backend/internal/auth"
	"mckit/backend/internal/captcha"
	"mckit/backend/internal/http/middleware"
	"mckit/backend/internal/service/user"
)

// newUserTestRouter 按 main.go 的接线方式搭建用户路由，用于鉴权 / CSRF 行为测试。
// 固定验证码 "135790"（生产由随机生成，测试经 FixedCode 钩子绕过图片识别）。
const testCaptchaCode = "135790"

func newUserTestRouter(t *testing.T) (chi.Router, *user.Service, auth.SessionStore) {
	t.Helper()
	repo := user.NewMemoryRepo()
	sessions := auth.NewMemorySessionStore()
	svc := user.NewService(repo, sessions, 5, 900)
	// 统一账号（迁移 0022）后管理员与普通用户共用会话 Cookie，处理器与中间件同名。
	middleware.SetCookieName("mc_test_session")
	h := NewUserHandler(svc, captcha.NewManager(captcha.Options{FixedCode: testCaptchaCode}), false)

	r := chi.NewRouter()
	r.Route("/user", func(ur chi.Router) {
		ur.Post("/captcha", h.Captcha)
		ur.Post("/register", h.Register)
		ur.Post("/login", h.Login)
		ur.Group(func(authed chi.Router) {
			authed.Use(middleware.RequireAuth(svc))
			authed.Use(middleware.RequireUserRole())
			authed.Post("/logout", h.Logout)
			authed.Post("/me", h.Me)
			authed.Post("/favorites/list", h.ListFavorites)
			authed.Group(func(csrf chi.Router) {
				csrf.Use(middleware.CSRF)
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
		req.AddCookie(&http.Cookie{Name: "mc_test_session", Value: cookie})
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
	u, err := svc.Register(ctx, user.RegisterInput{
		Username: "tester", Nickname: "测试者", Email: "tester@example.com", Password: "password123",
	})
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

// TestUserCaptchaEndpoint 图形验证码签发：返回一次性 ID 与 PNG data URI。
func TestUserCaptchaEndpoint(t *testing.T) {
	r, _, _ := newUserTestRouter(t)

	rec := doJSON(r, http.MethodPost, "/user/captcha", "", "", map[string]string{})
	if rec.Code != http.StatusOK {
		t.Fatalf("签发验证码期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			CaptchaID string `json:"captcha_id"`
			Image     string `json:"image"`
			ExpiresIn int    `json:"expires_in"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data.CaptchaID) != 32 {
		t.Errorf("captcha_id 应为 32 位十六进制，实际 %q", resp.Data.CaptchaID)
	}
	if !strings.HasPrefix(resp.Data.Image, "data:image/png;base64,") {
		t.Errorf("image 应为 PNG data URI，实际前缀 %q", resp.Data.Image[:min(24, len(resp.Data.Image))])
	}
	if resp.Data.ExpiresIn <= 0 {
		t.Errorf("expires_in 应为正，实际 %d", resp.Data.ExpiresIn)
	}
}

// TestUserRegisterRequiresCaptcha 注册必须携带正确验证码。
func TestUserRegisterRequiresCaptcha(t *testing.T) {
	r, _, _ := newUserTestRouter(t)

	body := map[string]string{
		"username": "captcha-user", "nickname": "验证码用户",
		"email": "captcha-user@example.com", "password": "password123",
	}

	// 缺验证码 → 422 CAPTCHA_INVALID
	rec := doJSON(r, http.MethodPost, "/user/register", "", "", body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("缺验证码期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "CAPTCHA_INVALID") {
		t.Errorf("应返回 CAPTCHA_INVALID 错误码: %s", rec.Body.String())
	}

	// 错误验证码 → 422
	bad := map[string]string{}
	for k, v := range body {
		bad[k] = v
	}
	bad["captcha_id"] = "whatever"
	bad["captcha_answer"] = "000000"
	rec = doJSON(r, http.MethodPost, "/user/register", "", "", bad)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("错误验证码期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 正确验证码 → 200，返回邮箱与昵称
	ok := map[string]string{}
	for k, v := range body {
		ok[k] = v
	}
	ok["captcha_id"] = "whatever"
	ok["captcha_answer"] = testCaptchaCode
	rec = doJSON(r, http.MethodPost, "/user/register", "", "", ok)
	if rec.Code != http.StatusOK {
		t.Fatalf("正确验证码期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "captcha-user@example.com") {
		t.Errorf("注册响应应包含邮箱: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "验证码用户") {
		t.Errorf("注册响应应包含昵称: %s", rec.Body.String())
	}
}

// TestUserLoginByEmail 用户名与邮箱均可登录。
func TestUserLoginByEmail(t *testing.T) {
	r, _, _ := newUserTestRouter(t)

	rec := doJSON(r, http.MethodPost, "/user/register", "", "", map[string]string{
		"username": "email-login", "nickname": "邮箱登录",
		"email": "email-login@example.com", "password": "password123",
		"captcha_id": "whatever", "captcha_answer": testCaptchaCode,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("注册期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	for _, id := range []string{"email-login", "email-login@example.com"} {
		rec = doJSON(r, http.MethodPost, "/user/login", "", "",
			map[string]string{"identifier": id, "password": "password123"})
		if rec.Code != http.StatusOK {
			t.Errorf("以 %q 登录期望 200，实际 %d（%s）", id, rec.Code, rec.Body.String())
		}
	}
}

func TestUserRoutesRejectAdminSession(t *testing.T) {
	r, _, sessions := newUserTestRouter(t)

	// 伪造一个管理员角色的会话塞入同一存储，并携带其 Cookie 访问用户接口。
	if err := sessions.Create(context.Background(), auth.Session{
		SessionID: "admin-sid",
		AccountID: "admin-1",
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
