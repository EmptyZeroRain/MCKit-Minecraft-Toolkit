package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestAdminRoutesRequireAuth 后台路由鉴权守卫测试（gj.md §16.1）：
//
// 除公开的登录 / 第二因子校验路由外，所有 /api/v1/admin/* 路由在无会话时
// 必须返回 401。新增后台路由若忘记挂 RequireAuth，本测试即失败——把「权限
// 边界」变成可自动验证的约束，而不是依赖人工检查。
func TestAdminRoutesRequireAuth(t *testing.T) {
	_, router := integrationSetup(t)

	mux, ok := router.(*chi.Mux)
	if !ok {
		t.Fatalf("路由根类型非 *chi.Mux: %T", router)
	}

	type route struct{ method, pattern string }
	var routes []route
	err := chi.Walk(mux, func(method, pattern string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method == http.MethodGet || method == "" {
			return nil
		}
		if !strings.HasPrefix(pattern, "/api/v1/admin/") {
			return nil
		}
		routes = append(routes, route{method, pattern})
		return nil
	})
	if err != nil {
		t.Fatalf("遍历路由失败: %v", err)
	}
	if len(routes) == 0 {
		t.Fatal("未发现任何 /api/v1/admin/* 路由，测试无效")
	}

	// 公开路由白名单：登录与第二因子校验（凭证是登录票据，不需要既有会话）。
	publicRoutes := map[string]bool{
		"/api/v1/admin/auth/login":      true,
		"/api/v1/admin/auth/mfa/verify": true,
	}
	for _, rt := range routes {
		if publicRoutes[rt.pattern] {
			continue
		}
		path := fillPathParams(rt.pattern)
		rec := do(t, router, rt.method, path, "{}", nil, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("路由 %s %s 未登录应返回 401，实际 %d", rt.method, rt.pattern, rec.Code)
		}
	}
}

// fillPathParams 把路径中的 {param} 段替换为固定占位值。
// RequireAuth 在业务处理器之前执行，因此占位值是否合法不影响鉴权断言。
func fillPathParams(pattern string) string {
	segs := strings.Split(pattern, "/")
	for i, s := range segs {
		if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
			segs[i] = "11111111-1111-1111-1111-111111111111"
		}
	}
	return strings.Join(segs, "/")
}
