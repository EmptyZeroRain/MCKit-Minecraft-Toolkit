package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"mckit/backend/internal/auth"
	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/middleware"
	"mckit/backend/internal/service/accountadmin"
)

// fixedPermResolver 固定返回一组权限点，用于测试处理器的二次鉴权。
type fixedPermResolver struct {
	perms []string
	err   error
}

func (f fixedPermResolver) PermissionsForRole(context.Context, string) ([]string, error) {
	return f.perms, f.err
}

// scopeRequest 构造携带指定权限点会话的请求。
func scopeRequest(t *testing.T, perms ...string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/list", nil)
	ctx := middleware.WithSession(req.Context(), &auth.Session{
		AccountID: "actor-1",
		Username:  "actor",
		Role:      "test-role",
	})
	return req.WithContext(ctx)
}

func newScopeHandler(perms ...string) *AccountAdminHandler {
	return (&AccountAdminHandler{}).WithPermissions(fixedPermResolver{perms: perms})
}

func TestEffectiveScope(t *testing.T) {
	tests := []struct {
		name       string
		perms      []string
		resolveErr error
		requested  string
		wantScope  string
		wantOK     bool
		wantCode   int
	}{
		{
			name:      "双权限默认全部",
			perms:     []string{domain.PermAccountsAdmins, domain.PermAccountsUsers},
			requested: "",
			wantScope: accountadmin.ScopeAll,
			wantOK:    true,
		},
		{
			name:      "仅普通用户权限收敛为普通用户",
			perms:     []string{domain.PermAccountsUsers},
			requested: "",
			wantScope: accountadmin.ScopeUser,
			wantOK:    true,
		},
		{
			name:      "仅普通用户权限请求后台账号被拒",
			perms:     []string{domain.PermAccountsUsers},
			requested: accountadmin.ScopeAdmin,
			wantOK:    false,
			wantCode:  http.StatusForbidden,
		},
		{
			name:      "仅后台权限收敛为后台账号",
			perms:     []string{domain.PermAccountsAdmins},
			requested: "",
			wantScope: accountadmin.ScopeAdmin,
			wantOK:    true,
		},
		{
			name:      "仅后台权限请求普通用户被拒",
			perms:     []string{domain.PermAccountsAdmins},
			requested: accountadmin.ScopeUser,
			wantOK:    false,
			wantCode:  http.StatusForbidden,
		},
		{
			name:      "无任何账号权限被拒",
			perms:     nil,
			requested: "",
			wantOK:    false,
			wantCode:  http.StatusForbidden,
		},
		{
			name:       "权限解析失败按无权处理",
			resolveErr: errors.New("db down"),
			requested:  "",
			wantOK:     false,
			wantCode:   http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := (&AccountAdminHandler{}).WithPermissions(fixedPermResolver{perms: tt.perms, err: tt.resolveErr})
			rec := httptest.NewRecorder()
			scope, ok := h.effectiveScope(rec, scopeRequest(t), tt.requested)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v，期望 %v（响应 %d）", ok, tt.wantOK, rec.Code)
			}
			if ok && scope != tt.wantScope {
				t.Fatalf("scope=%q，期望 %q", scope, tt.wantScope)
			}
			if !ok && tt.wantCode != 0 && rec.Code != tt.wantCode {
				t.Fatalf("状态码=%d，期望 %d", rec.Code, tt.wantCode)
			}
		})
	}
}

func TestRequireTargetPermission(t *testing.T) {
	tests := []struct {
		name     string
		perms    []string
		role     string
		wantPass bool
	}{
		{"普通用户 + accounts.users", []string{domain.PermAccountsUsers}, domain.RoleUser, true},
		{"普通用户 + 仅 accounts.admins", []string{domain.PermAccountsAdmins}, domain.RoleUser, false},
		{"后台角色 + accounts.admins", []string{domain.PermAccountsAdmins}, "editor", true},
		{"后台角色 + 仅 accounts.users", []string{domain.PermAccountsUsers}, "editor", false},
		{"后台角色 + 双权限", []string{domain.PermAccountsAdmins, domain.PermAccountsUsers}, "admin", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newScopeHandler(tt.perms...)
			rec := httptest.NewRecorder()
			got := h.requireTargetPermission(rec, scopeRequest(t), tt.role)
			if got != tt.wantPass {
				t.Fatalf("pass=%v，期望 %v（响应 %d）", got, tt.wantPass, rec.Code)
			}
			if !got && rec.Code != http.StatusForbidden {
				t.Fatalf("拒绝时状态码=%d，期望 403", rec.Code)
			}
		})
	}
}
