package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mckit/backend/internal/http/middleware"
)

// TestAdminLoginHistoryIntegration 覆盖「用户信息 → 登录历史」：
//   - 登录时记录的设备（User-Agent）与来源 IP 能在本人登录历史里取回；
//   - 接口只要求已登录，不要求 ops.audit 权限；
//   - 未登录返回 401。
func TestAdminLoginHistoryIntegration(t *testing.T) {
	_, router := integrationSetup(t)

	const agent = "it-agent/1.0 (login-history-test)"
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, itAdminUser, itPassword)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", agent)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("登录失败：%d %s", rec.Code, rec.Body.String())
	}
	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == middleware.SessionCookieName() {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("登录响应缺少会话 Cookie")
	}

	// 未登录必须 401：登录历史是账号私有数据。
	if unauth := do(t, router, http.MethodPost, "/api/v1/admin/auth/login-history", `{}`, nil, ""); unauth.Code != http.StatusUnauthorized {
		t.Fatalf("未登录期望 401，实际 %d", unauth.Code)
	}

	history := do(t, router, http.MethodPost, "/api/v1/admin/auth/login-history", `{}`, cookie, "")
	if history.Code != http.StatusOK {
		t.Fatalf("登录历史期望 200，实际 %d（%s）", history.Code, history.Body.String())
	}
	var resp struct {
		Data struct {
			Items []struct {
				CreatedAt string `json:"created_at"`
				SourceIP  string `json:"source_ip"`
				UserAgent string `json:"user_agent"`
				Result    string `json:"result"`
			} `json:"items"`
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(history.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析登录历史失败: %v", err)
	}
	if resp.Data.Total < 1 || len(resp.Data.Items) < 1 {
		t.Fatalf("登录后应至少有 1 条记录，实际 total=%d", resp.Data.Total)
	}
	item := resp.Data.Items[0]
	if item.Result != "success" {
		t.Fatalf("最新登录结果期望 success，实际 %q", item.Result)
	}
	if item.UserAgent != agent {
		t.Fatalf("登录设备的 User-Agent 未被记录：%q", item.UserAgent)
	}
	if item.SourceIP == "" {
		t.Fatal("登录历史缺少来源 IP")
	}
	if item.CreatedAt == "" {
		t.Fatal("登录历史缺少登录时间")
	}
}
