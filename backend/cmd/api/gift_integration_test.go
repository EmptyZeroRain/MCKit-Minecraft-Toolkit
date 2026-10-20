package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"mckit/backend/internal/domain"
)

// TestGiftCenterAPI 覆盖礼物中心路由装配：签到、目录、兑换、后台目录与双币调整。
func TestGiftCenterAPI(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, gdb)
	if err := gdb.Exec("UPDATE gift_settings SET checkin_reward = 1").Error; err != nil {
		t.Fatalf("重置签到奖励失败: %v", err)
	}

	const userPass = "gift-user-pass-123"
	username := fmt.Sprintf("it-gift-%d", time.Now().UnixNano())

	rec := do(t, router, http.MethodPost, "/api/v1/user/register",
		fmt.Sprintf(`{"username":%q,"nickname":"礼物测试","email":%q,"password":%q,"captcha_id":"it","captcha_answer":"135790"}`,
			username, username+"@example.com", userPass), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("注册期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	rec = do(t, router, http.MethodPost, "/api/v1/user/login",
		fmt.Sprintf(`{"identifier":%q,"password":%q}`, username, userPass), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("登录期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var loginResp struct {
		Data struct {
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("解析登录响应失败: %v", err)
	}
	var userCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "mc_session" {
			userCookie = c
		}
	}
	if userCookie == nil || loginResp.Data.CSRFToken == "" {
		t.Fatal("登录响应缺少会话 Cookie 或 CSRF token")
	}
	csrf := loginResp.Data.CSRFToken

	var acc domain.Account
	if err := gdb.Where("username = ?", username).First(&acc).Error; err != nil {
		t.Fatalf("查询测试账号失败: %v", err)
	}
	targetID := acc.ID.String()

	// 读接口无需 CSRF。
	rec = do(t, router, http.MethodPost, "/api/v1/gifts/overview", `{}`, userCookie, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("钱包概览期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 签到是写操作：缺 CSRF 必须 403。
	rec = do(t, router, http.MethodPost, "/api/v1/gifts/checkin", `{}`, userCookie, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("签到缺 CSRF 期望 403，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/gifts/checkin", `{}`, userCookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("签到期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/gifts/checkin", `{}`, userCookie, csrf)
	if rec.Code != http.StatusConflict {
		t.Fatalf("重复签到期望 409，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 目录取泥土块。
	rec = do(t, router, http.MethodPost, "/api/v1/gifts/catalog", `{}`, userCookie, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("礼物目录期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var catalogResp struct {
		Data []struct {
			ID    string `json:"id"`
			Code  string `json:"code"`
			Price int64  `json:"price"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &catalogResp); err != nil {
		t.Fatalf("解析礼物目录失败: %v", err)
	}
	var dirtID string
	for _, item := range catalogResp.Data {
		if item.Code == "dirt_block" {
			dirtID = item.ID
		}
	}
	if dirtID == "" {
		t.Fatal("初始礼物目录缺少 dirt_block")
	}

	rec = do(t, router, http.MethodPost, "/api/v1/gifts/redeem",
		fmt.Sprintf(`{"gift_id":%q,"request_key":"it-redeem-1"}`, dirtID), userCookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("兑换期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 后台：目录 CRUD + 签到配置 + 签到币调整。
	adminCookie, adminCSRF := loginAs(t, router, itAdminUser)
	rec = do(t, router, http.MethodPost, "/api/v1/admin/gifts/definitions/list", `{}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("后台礼物目录期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	rec = do(t, router, http.MethodPost, "/api/v1/admin/gifts/definitions/create",
		`{"code":"it_api_gift","name":"接口测试礼物","description":"test","icon":"🧪","price":2,"allow_checkin":true,"sort_order":99,"enabled":true}`,
		adminCookie, adminCSRF)
	if rec.Code != http.StatusCreated {
		t.Fatalf("新建礼物期望 201，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	created := decodeData(t, rec)
	giftID, _ := created["id"].(string)
	if giftID == "" {
		t.Fatal("新建礼物响应缺少 id")
	}

	rec = do(t, router, http.MethodPost, "/api/v1/admin/gifts/definitions/update",
		fmt.Sprintf(`{"id":%q,"name":"接口测试礼物（改）","description":"test2","icon":"🧪","price":3,"allow_checkin":false,"sort_order":100,"enabled":false}`, giftID),
		adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("编辑礼物期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/gifts/definitions/delete",
		fmt.Sprintf(`{"id":%q}`, giftID), adminCookie, adminCSRF)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("删除礼物期望 204，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	rec = do(t, router, http.MethodPost, "/api/v1/admin/gifts/settings/update",
		`{"checkin_reward":3}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("更新签到奖励期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/gifts/settings/get", `{}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("读取签到奖励期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	if got := decodeData(t, rec)["checkin_reward"].(float64); got != 3 {
		t.Fatalf("签到奖励期望 3，实际 %v", got)
	}

	rec = do(t, router, http.MethodPost, "/api/v1/admin/gifts/wallet/adjust",
		fmt.Sprintf(`{"target_user_id":%q,"coin_type":"checkin","amount":10,"reason":"接口测试","request_key":"it-adjust-checkin"}`, targetID),
		adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("调整签到币期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/gifts/wallet/get",
		fmt.Sprintf(`{"target_user_id":%q}`, targetID), adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("查询钱包期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	wallet := decodeData(t, rec)
	// 签到 1 币，兑换泥土块消耗 1 签到币，后台再发 10 签到币。
	if got := wallet["checkin_balance"].(float64); got != 10 {
		t.Fatalf("签到币余额期望 10，实际 %v", got)
	}
	if got := wallet["general_balance"].(float64); got != 0 {
		t.Fatalf("通用币余额期望 0，实际 %v", got)
	}

	// 三类排行榜接口均可访问，且返回分页 meta。
	for _, path := range []string{
		"/api/v1/admin/gifts/rankings/checkin",
		"/api/v1/admin/gifts/rankings/gifts",
		"/api/v1/admin/gifts/rankings/coins",
	} {
		rec = do(t, router, http.MethodPost, path, `{"days":0,"page":1,"page_size":10}`, adminCookie, adminCSRF)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s 期望 200，实际 %d（%s）", path, rec.Code, rec.Body.String())
		}
		var rankingResp struct {
			Data []any `json:"data"`
			Meta struct {
				Total int64 `json:"total"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &rankingResp); err != nil {
			t.Fatalf("解析 %s 响应失败: %v", path, err)
		}
		if len(rankingResp.Data) == 0 {
			t.Fatalf("%s 期望至少一条数据", path)
		}
	}
}
