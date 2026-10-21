package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/middleware"
)

// TestAdminLoginHistoryIntegration 覆盖「用户信息 → 登录历史」：
//   - 登录时记录的设备（User-Agent）与来源 IP 能在本人登录历史里取回；
//   - 接口只要求已登录，不要求 ops.audit 权限；
//   - 未登录返回 401。
func TestAdminLoginHistoryIntegration(t *testing.T) {
	gdb, router := integrationSetup(t)

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
			Total    int `json:"total"`
			Page     int `json:"page"`
			PageSize int `json:"page_size"`
		} `json:"data"`
	}
	if err := json.Unmarshal(history.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析登录历史失败: %v", err)
	}
	if resp.Data.Total < 1 || len(resp.Data.Items) < 1 {
		t.Fatalf("登录后应至少有 1 条记录，实际 total=%d", resp.Data.Total)
	}
	// 默认第 1 页、每页 10 条。
	if resp.Data.Page != 1 || resp.Data.PageSize != 10 {
		t.Fatalf("默认分页参数不符：page=%d page_size=%d", resp.Data.Page, resp.Data.PageSize)
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

	// 默认每页 10 条：补 12 条登录审计后，第一页只返回 10 条，第二页仍可翻。
	var acc domain.Account
	if err := gdb.Where("username = ?", itAdminUser).First(&acc).Error; err != nil {
		t.Fatalf("查询管理员账号失败: %v", err)
	}
	for i := 0; i < 12; i++ {
		row := domain.AuditLog{
			ActorAdminID:  &acc.ID,
			Action:        "admin.login",
			ObjectType:    "admin",
			Result:        domain.AuditSuccess,
			SourceIP:      fmt.Sprintf("192.0.2.%d", i+1),
			BeforeSummary: "{}",
			AfterSummary:  `{"user_agent":"page-test"}`,
		}
		if err := gdb.Create(&row).Error; err != nil {
			t.Fatalf("写入登录审计失败: %v", err)
		}
	}
	fetch := func(body string) []string {
		t.Helper()
		rec := do(t, router, http.MethodPost, "/api/v1/admin/auth/login-history", body, cookie, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("登录历史期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
		}
		var page struct {
			Data struct {
				Items []struct {
					SourceIP string `json:"source_ip"`
				} `json:"items"`
				Total    int `json:"total"`
				Page     int `json:"page"`
				PageSize int `json:"page_size"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatalf("解析登录历史失败: %v", err)
		}
		wantPage := 1
		if strings.Contains(body, `"page":2`) {
			wantPage = 2
		}
		if page.Data.Page != wantPage {
			t.Fatalf("页码回显不符：%+v", page.Data)
		}
		if page.Data.PageSize != 10 || page.Data.Total < 12 {
			t.Fatalf("分页参数不符：%+v", page.Data)
		}
		ips := make([]string, 0, len(page.Data.Items))
		for _, it := range page.Data.Items {
			ips = append(ips, it.SourceIP)
		}
		return ips
	}
	p1 := fetch(`{"page":1,"page_size":10}`)
	p2 := fetch(`{"page":2,"page_size":10}`)
	if len(p1) != 10 {
		t.Fatalf("第一页期望 10 条，实际 %d", len(p1))
	}
	if len(p2) == 0 || len(p2) > 10 {
		t.Fatalf("第二页条数异常：%d", len(p2))
	}
	seen := map[string]bool{}
	for _, ip := range p1 {
		seen[ip] = true
	}
	for _, ip := range p2 {
		if seen[ip] {
			t.Fatalf("第二页与第一页出现重复记录：%s", ip)
		}
	}
}
