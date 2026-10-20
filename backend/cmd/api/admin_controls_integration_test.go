package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"mckit/backend/internal/auth"
	"mckit/backend/internal/domain"
)

// TestGrowthAdminControlsAndGiftAPI 覆盖新增的后台等级调整、成就授予 / 撤销与礼物钱包接口。
func TestGrowthAdminControlsAndGiftAPI(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	hash, err := auth.HashPassword(itPassword)
	if err != nil {
		t.Fatalf("生成测试密码失败: %v", err)
	}
	user := domain.User{
		Username: "it-controls-user", UserNo: "UCTRL000001",
		PasswordHash: hash, Role: domain.RoleUser, IsActive: true,
	}
	if err := gdb.Create(&user).Error; err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}
	var achievement domain.AchievementDefinition
	if err := gdb.Where("code = ?", "level_3").First(&achievement).Error; err != nil {
		t.Fatalf("读取测试成就失败: %v", err)
	}

	cookie, csrf := loginAs(t, router, itAdminUser)

	// 1) 单用户等级 +2。
	rec := do(t, router, http.MethodPost, "/api/v1/admin/growth/levels/adjust",
		fmt.Sprintf(`{"target_user_id":%q,"delta_level":2,"reason":"接口集成测试"}`, user.UserNo),
		cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("等级调整失败: %d %s", rec.Code, rec.Body.String())
	}
	data := decodeData(t, rec)
	var gotLevel, gotOffset int
	if v, ok := data["level"].(float64); ok {
		gotLevel = int(v)
	}
	if v, ok := data["level_offset"].(float64); ok {
		gotOffset = int(v)
	}
	if gotLevel != 3 || gotOffset != 2 {
		t.Fatalf("等级调整结果异常: level=%d offset=%d body=%s", gotLevel, gotOffset, rec.Body.String())
	}

	// 2) 授予成就 -> 查询 -> 撤销。
	grantBody := fmt.Sprintf(`{"target_user_id":%q,"achievement_id":%q,"reason":"接口授予"}`,
		user.ID.String(), achievement.ID.String())
	rec = do(t, router, http.MethodPost, "/api/v1/admin/achievements/grant", grantBody, cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("授予成就失败: %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/achievements/user/list",
		fmt.Sprintf(`{"target_user_id":%q}`, user.UserNo), cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("查询用户成就失败: %d %s", rec.Code, rec.Body.String())
	}
	var listResp struct {
		Data struct {
			Items []struct {
				Code     string `json:"code"`
				Unlocked bool   `json:"unlocked"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("解析用户成就失败: %v", err)
	}
	found := false
	for _, item := range listResp.Data.Items {
		if item.Code == achievement.Code && item.Unlocked {
			found = true
		}
	}
	if !found {
		t.Fatalf("用户成就列表未显示已授予成就: %s", rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/achievements/revoke", grantBody, cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("撤销成就失败: %d %s", rec.Code, rec.Body.String())
	}

	// 3) 礼物钱包 +50 -> 查询余额。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/gifts/wallet/adjust",
		fmt.Sprintf(`{"target_user_id":%q,"amount":50,"reason":"接口测试发放","request_key":"it-gift-1"}`, user.UserNo),
		cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("礼物币调整失败: %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/gifts/wallet/get",
		fmt.Sprintf(`{"target_user_id":%q}`, user.UserNo), cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("礼物钱包查询失败: %d %s", rec.Code, rec.Body.String())
	}
	wallet := decodeData(t, rec)
	if v, ok := wallet["total_balance"].(float64); !ok || int(v) != 50 {
		t.Fatalf("礼物钱包总余额异常: %s", rec.Body.String())
	}
	if v, ok := wallet["general_balance"].(float64); !ok || int(v) != 50 {
		t.Fatalf("礼物钱包通用币异常: %s", rec.Body.String())
	}
	if v, ok := wallet["checkin_balance"].(float64); !ok || int(v) != 0 {
		t.Fatalf("礼物钱包签到币异常: %s", rec.Body.String())
	}

	// 4) 签到币与通用币分账。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/gifts/wallet/adjust",
		fmt.Sprintf(`{"target_user_id":%q,"coin_type":"checkin","amount":20,"reason":"接口测试签到币","request_key":"it-gift-2"}`, user.UserNo),
		cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("签到币调整失败: %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/gifts/wallet/get",
		fmt.Sprintf(`{"target_user_id":%q}`, user.UserNo), cookie, csrf)
	wallet = decodeData(t, rec)
	if v, ok := wallet["total_balance"].(float64); !ok || int(v) != 70 {
		t.Fatalf("双币种总余额异常: %s", rec.Body.String())
	}
	if v, ok := wallet["checkin_balance"].(float64); !ok || int(v) != 20 {
		t.Fatalf("签到币余额异常: %s", rec.Body.String())
	}

	// 5) editor 无 ops.gifts，应 403。
	editorCookie, editorCSRF := loginAs(t, router, itEditorUser)
	rec = do(t, router, http.MethodPost, "/api/v1/admin/gifts/wallet/adjust",
		fmt.Sprintf(`{"target_user_id":%q,"amount":1,"reason":"越权测试"}`, user.ID.String()),
		editorCookie, editorCSRF)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("editor 访问礼物钱包期望 403，实际 %d %s", rec.Code, rec.Body.String())
	}
}
