package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// TestAdminSessionManagementIntegration 覆盖系统管理 → 会话管理全链路：
//   - 管理员可同时看到普通用户与管理员会话；
//   - 按展示指纹踢下线普通用户后，目标会话立即 401；
//   - 当前管理员自己的会话禁止在列表中踢下线。
func TestAdminSessionManagementIntegration(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, gdb)

	const username = "it-session-user"
	registerUser(t, router, username)
	userCookie, _ := loginUser(t, router, username)
	adminCookie, adminCSRF := loginAs(t, router, itAdminUser)

	list := func(filter string) (items []struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Current  bool   `json:"current"`
	}, total int64) {
		rec := do(t, router, http.MethodPost, "/api/v1/admin/sessions/list",
			filter, adminCookie, adminCSRF)
		if rec.Code != http.StatusOK {
			t.Fatalf("会话列表期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
		}
		var resp struct {
			Data []struct {
				ID       string `json:"id"`
				Username string `json:"username"`
				Current  bool   `json:"current"`
			} `json:"data"`
			Meta struct {
				Total int64 `json:"total"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("解析会话列表失败: %v", err)
		}
		return resp.Data, resp.Meta.Total
	}

	items, total := list(fmt.Sprintf(`{"q":%q,"account_type":"user"}`, username))
	if total != 1 || len(items) != 1 {
		t.Fatalf("普通用户会话应命中 1 条，实际 total=%d items=%+v", total, items)
	}
	targetFingerprint := items[0].ID
	if len(targetFingerprint) != 16 {
		t.Fatalf("会话展示指纹长度异常: %q", targetFingerprint)
	}

	rec := do(t, router, http.MethodPost, "/api/v1/admin/sessions/revoke",
		fmt.Sprintf(`{"id":%q}`, targetFingerprint), adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("踢下线期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/user/me", `{}`, userCookie, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("被踢下线的用户会话应 401，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	if items, total := list(fmt.Sprintf(`{"q":%q,"account_type":"user"}`, username)); total != 0 || len(items) != 0 {
		t.Fatalf("踢下线后普通用户会话应清空，实际 total=%d items=%+v", total, items)
	}

	// 当前管理员会话：列表标记 current，撤销应被拒绝。
	// 其他用例可能已留下同账号会话，这里只要求本次登录的会话被标记为 current。
	adminItems, adminTotal := list(fmt.Sprintf(`{"q":%q,"account_type":"admin"}`, itAdminUser))
	if adminTotal < 1 || len(adminItems) == 0 {
		t.Fatalf("管理员会话列表为空: total=%d items=%+v", adminTotal, adminItems)
	}
	currentID := ""
	for _, item := range adminItems {
		if item.Username != itAdminUser {
			t.Fatalf("管理员会话过滤异常: %+v", item)
		}
		if item.Current {
			currentID = item.ID
		}
	}
	if currentID == "" {
		t.Fatalf("未找到当前管理员会话: %+v", adminItems)
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/sessions/revoke",
		fmt.Sprintf(`{"id":%q}`, currentID), adminCookie, adminCSRF)
	if rec.Code != http.StatusConflict {
		t.Fatalf("撤销当前会话期望 409，实际 %d（%s）", rec.Code, rec.Body.String())
	}
}
