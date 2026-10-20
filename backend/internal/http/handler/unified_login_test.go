package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"mckit/backend/internal/auth"
	"mckit/backend/internal/config"
	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/middleware"
	"mckit/backend/internal/service/user"
)

// unifiedFakeAdminRepo 内存版后台账号仓库（统一登录测试专用）。
type unifiedFakeAdminRepo struct {
	byName map[string]*domain.Admin
	byID   map[string]*domain.Admin
}

func (r *unifiedFakeAdminRepo) FindByUsername(username string) (*domain.Admin, error) {
	return r.byName[username], nil
}

func (r *unifiedFakeAdminRepo) FindByID(id string) (*domain.Admin, error) {
	return r.byID[id], nil
}

func (r *unifiedFakeAdminRepo) UpdateLastLogin(string) error { return nil }

func (r *unifiedFakeAdminRepo) ChangePassword(string, string) error { return nil }

func (r *unifiedFakeAdminRepo) UpdateProfile(string, string, string) error { return nil }

// unifiedFakeMFA 已启用 MFA 的认证能力替身。
type unifiedFakeMFA struct{ accountID uuid.UUID }

func (f *unifiedFakeMFA) IsEnabled(context.Context, uuid.UUID) (bool, error) { return true, nil }

func (f *unifiedFakeMFA) CreateTicket(context.Context, uuid.UUID) (string, error) {
	return "ticket-unified", nil
}

func (f *unifiedFakeMFA) RedeemTicket(context.Context, string, string) (uuid.UUID, error) {
	return f.accountID, nil
}

func (f *unifiedFakeMFA) Verify(context.Context, uuid.UUID, string) error { return nil }

// unifiedGrowthSink 记录统一登录投递的成长事件。
type unifiedGrowthSink struct {
	events []domain.GrowthEvent
}

func (s *unifiedGrowthSink) EmitGrowthEvent(_ context.Context, event domain.GrowthEvent) error {
	s.events = append(s.events, event)
	return nil
}

type unifiedLoginFixture struct {
	router   chi.Router
	sessions auth.SessionStore
	admin    *domain.Admin
	sink     *unifiedGrowthSink
	authSvc  *auth.Service
}

// newUnifiedLoginFixture 按 main.go 的接线方式搭建统一登录路由。
func newUnifiedLoginFixture(t *testing.T) *unifiedLoginFixture {
	t.Helper()
	middleware.SetCookieName("mc_test_session")

	repo := user.NewMemoryRepo()
	sessions := auth.NewMemorySessionStore()
	userSvc := user.NewService(repo, sessions, 5, 900)

	ctx := context.Background()
	if _, err := userSvc.Register(ctx, user.RegisterInput{
		Username: "alice",
		Nickname: "爱丽丝",
		Email:    "alice@example.com",
		Password: "password123",
	}); err != nil {
		t.Fatalf("创建普通用户失败: %v", err)
	}

	adminID := uuid.New()
	adminHash, err := auth.HashPassword("AdminPass123!")
	if err != nil {
		t.Fatalf("生成管理员密码哈希失败: %v", err)
	}
	admin := &domain.Admin{
		ID:           adminID,
		Username:     "rootadmin",
		Email:        "root@example.com",
		PasswordHash: adminHash,
		Role:         domain.RoleAdmin,
		IsActive:     true,
	}
	if err := repo.Create(ctx, admin); err != nil {
		t.Fatalf("创建管理员账号失败: %v", err)
	}

	authSvc := auth.NewService(&unifiedFakeAdminRepo{
		byName: map[string]*domain.Admin{admin.Username: admin},
		byID:   map[string]*domain.Admin{admin.ID.String(): admin},
	}, sessions, 5, 900)
	sink := &unifiedGrowthSink{}
	h := NewAuthHandler(authSvc, sessions, &config.Config{}).
		WithUserService(userSvc).
		WithGrowthSink(sink)

	r := chi.NewRouter()
	r.Post("/auth/login", h.UnifiedLogin)
	return &unifiedLoginFixture{router: r, sessions: sessions, admin: admin, sink: sink, authSvc: authSvc}
}

type unifiedLoginResponse struct {
	Data struct {
		Role     string `json:"role"`
		Username string `json:"username"`
	} `json:"data"`
	Error *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func doUnifiedLogin(t *testing.T, r chi.Router, identifier, password string) (*httptest.ResponseRecorder, unifiedLoginResponse) {
	t.Helper()
	rec := doJSON(r, http.MethodPost, "/auth/login", "", "", map[string]string{
		"identifier": identifier,
		"password":   password,
	})
	var body unifiedLoginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v（原始响应：%s）", err, rec.Body.String())
	}
	return rec, body
}

func TestUnifiedLoginNormalUser(t *testing.T) {
	f := newUnifiedLoginFixture(t)
	rec, body := doUnifiedLogin(t, f.router, "alice", "password123")
	if rec.Code != http.StatusOK {
		t.Fatalf("普通用户登录应成功，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if body.Data.Role != domain.RoleUser {
		t.Errorf("role = %q，期望 %q", body.Data.Role, domain.RoleUser)
	}
	if len(f.sink.events) != 1 || f.sink.events[0].EventType != domain.EventUserLoginSucceeded {
		t.Errorf("普通用户登录应投递 1 个登录事件，实际 %+v", f.sink.events)
	}
	if cookies := rec.Result().Cookies(); len(cookies) == 0 || cookies[0].Value == "" {
		t.Error("登录成功应种下会话 Cookie")
	}
}

func TestUnifiedLoginAdmin(t *testing.T) {
	f := newUnifiedLoginFixture(t)
	rec, body := doUnifiedLogin(t, f.router, "rootadmin", "AdminPass123!")
	if rec.Code != http.StatusOK {
		t.Fatalf("后台账号登录应成功，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if body.Data.Role != domain.RoleAdmin {
		t.Errorf("role = %q，期望 %q", body.Data.Role, domain.RoleAdmin)
	}
	if len(f.sink.events) != 0 {
		t.Errorf("后台账号登录不应投递普通用户登录事件，实际 %+v", f.sink.events)
	}
}

func TestUnifiedLoginAdminByEmail(t *testing.T) {
	f := newUnifiedLoginFixture(t)
	rec, body := doUnifiedLogin(t, f.router, "root@example.com", "AdminPass123!")
	if rec.Code != http.StatusOK {
		t.Fatalf("后台账号用邮箱登录应成功，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if body.Data.Role != domain.RoleAdmin {
		t.Errorf("role = %q，期望 %q", body.Data.Role, domain.RoleAdmin)
	}
}

func TestUnifiedLoginWrongPassword(t *testing.T) {
	f := newUnifiedLoginFixture(t)
	for _, identifier := range []string{"alice", "rootadmin", "root@example.com"} {
		rec, _ := doUnifiedLogin(t, f.router, identifier, "wrong-password")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s 密码错误应返回 401，实际 %d", identifier, rec.Code)
		}
	}
}

func TestUnifiedLoginUnknownAccount(t *testing.T) {
	f := newUnifiedLoginFixture(t)
	rec, _ := doUnifiedLogin(t, f.router, "nobody", "whatever123")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未知账号应返回 401，实际 %d", rec.Code)
	}
}

func TestUnifiedLoginAdminMFAChallenge(t *testing.T) {
	f := newUnifiedLoginFixture(t)
	f.authSvc.WithMFA(&unifiedFakeMFA{accountID: f.admin.ID})

	rec := doJSON(f.router, http.MethodPost, "/auth/login", "", "", map[string]string{
		"identifier": "rootadmin",
		"password":   "AdminPass123!",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("启用 MFA 的后台账号密码校验应成功，实际 %d：%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Data struct {
			MFARequired bool   `json:"mfa_required"`
			MFATicket   string `json:"mfa_ticket"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("解析 MFA 响应失败: %v", err)
	}
	if !payload.Data.MFARequired || payload.Data.MFATicket == "" {
		t.Errorf("应返回 MFA 票据，实际 %+v", payload.Data)
	}
	if cookies := rec.Result().Cookies(); len(cookies) != 0 && cookies[0].Value != "" {
		t.Error("MFA 未完成前不应种下会话 Cookie")
	}
}
