package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"mckit/backend/internal/auth"
	"mckit/backend/internal/domain"
	mfacrypto "mckit/backend/internal/mfa"
)

// TestMFAEnrollAndLoginFlow 覆盖双因子认证的完整 HTTP 链路（gj.md §10.1）：
//
//	绑定（setup → enable，拿到恢复码）
//	→ 登录返回 mfa_required + 票据（不种 Cookie）
//	→ 错误验证码 401
//	→ 正确验证码建立会话
//	→ 恢复码可登录一次
//	→ 解绑需要密码 + 验证码
//
// 使用独立账号，避免污染其他用例共享的 it-admin。
func TestMFAEnrollAndLoginFlow(t *testing.T) {
	gdb, router := integrationSetup(t)
	ctx := context.Background()

	const (
		username = "it-mfa"
		password = "mfa-integration-pass-123"
	)

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("生成密码哈希失败: %v", err)
	}
	acc := domain.Account{
		Username:     username,
		PasswordHash: hash,
		Role:         domain.RoleAdmin,
		IsActive:     true,
	}
	if err := gdb.WithContext(ctx).Create(&acc).Error; err != nil {
		t.Fatalf("创建测试账号失败: %v", err)
	}
	// 清理账号（MFA 表随外键级联删除）。
	t.Cleanup(func() {
		gdb.WithContext(context.Background()).Where("id = ?", acc.ID).Delete(&domain.Account{})
	})

	// ---- 1. 登录并绑定 MFA ----
	cookie, csrf := loginWithPassword(t, router, username, password)

	rec := do(t, router, http.MethodPost, "/api/v1/admin/auth/mfa/setup", "{}", cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup 失败：%d %s", rec.Code, rec.Body.String())
	}
	var setupResp struct {
		Data struct {
			Secret string `json:"secret"`
			URI    string `json:"uri"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &setupResp); err != nil {
		t.Fatalf("解析 setup 响应失败: %v", err)
	}
	if setupResp.Data.Secret == "" {
		t.Fatal("setup 应返回 secret")
	}

	code, err := mfacrypto.CodeAt(setupResp.Data.Secret, time.Now())
	if err != nil {
		t.Fatalf("计算验证码失败: %v", err)
	}

	rec = do(t, router, http.MethodPost, "/api/v1/admin/auth/mfa/enable",
		`{"code":"`+code+`"}`, cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable 失败：%d %s", rec.Code, rec.Body.String())
	}
	var enableResp struct {
		Data struct {
			RecoveryCodes []string `json:"recovery_codes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &enableResp); err != nil {
		t.Fatalf("解析 enable 响应失败: %v", err)
	}
	if len(enableResp.Data.RecoveryCodes) != mfacrypto.RecoveryCodeCount {
		t.Fatalf("恢复码数量 = %d, want %d", len(enableResp.Data.RecoveryCodes), mfacrypto.RecoveryCodeCount)
	}

	// ---- 2. 再次登录：应要求第二因子，且不种 Cookie ----
	rec = do(t, router, http.MethodPost, "/api/v1/admin/auth/login",
		`{"username":"`+username+`","password":"`+password+`"}`, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("登录失败：%d %s", rec.Code, rec.Body.String())
	}
	var loginResp struct {
		Data struct {
			MFARequired bool   `json:"mfa_required"`
			MFATicket   string `json:"mfa_ticket"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("解析登录响应失败: %v", err)
	}
	if !loginResp.Data.MFARequired || loginResp.Data.MFATicket == "" {
		t.Fatalf("应返回 mfa_required + 票据，实际 %s", rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "mc_session" && c.Value != "" {
			t.Fatal("第二因子通过前不应种下会话 Cookie")
		}
	}

	// ---- 3. 错误验证码 ----
	rec = do(t, router, http.MethodPost, "/api/v1/admin/auth/mfa/verify",
		`{"ticket":"`+loginResp.Data.MFATicket+`","code":"000000"}`, nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("错误验证码应返回 401，实际 %d %s", rec.Code, rec.Body.String())
	}

	// ---- 4. 正确验证码建立会话 ----
	// Enable 已消费当前时间步，因此这里用「下一步」的验证码（校验窗口 ±1 步）。
	next := time.Now().Add(mfacrypto.Period * time.Second)
	code2, err := mfacrypto.CodeAt(setupResp.Data.Secret, next)
	if err != nil {
		t.Fatal(err)
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/auth/mfa/verify",
		`{"ticket":"`+loginResp.Data.MFATicket+`","code":"`+code2+`"}`, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("正确验证码应登录成功，实际 %d %s", rec.Code, rec.Body.String())
	}
	var mfaCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "mc_session" && c.Value != "" {
			mfaCookie = c
		}
	}
	if mfaCookie == nil {
		t.Fatal("第二因子通过后应种下会话 Cookie")
	}

	// 会话可用：/me 返回该账号。
	meRec := do(t, router, http.MethodPost, "/api/v1/admin/auth/me", "{}", mfaCookie, "")
	if meRec.Code != http.StatusOK {
		t.Fatalf("新会话应可用，实际 %d %s", meRec.Code, meRec.Body.String())
	}

	// ---- 5. 恢复码可登录一次（第二次失效）----
	recovery := enableResp.Data.RecoveryCodes[0]
	rec = do(t, router, http.MethodPost, "/api/v1/admin/auth/mfa/verify",
		`{"ticket":"`+loginResp.Data.MFATicket+`","code":"`+recovery+`"}`, nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("已兑换的票据不可重复使用，实际 %d", rec.Code)
	}

	rec = do(t, router, http.MethodPost, "/api/v1/admin/auth/login",
		`{"username":"`+username+`","password":"`+password+`"}`, nil, "")
	var second struct {
		Data struct {
			MFATicket string `json:"mfa_ticket"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &second)
	if second.Data.MFATicket == "" {
		t.Fatalf("应再次返回票据: %s", rec.Body.String())
	}

	rec = do(t, router, http.MethodPost, "/api/v1/admin/auth/mfa/verify",
		`{"ticket":"`+second.Data.MFATicket+`","code":"`+recovery+`"}`, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("恢复码首次使用应成功，实际 %d %s", rec.Code, rec.Body.String())
	}

	// ---- 6. 解绑：密码错误被拒绝，正确时成功 ----
	// 恢复码不依赖时间步，用它建立会话，避免测试对 30 秒窗口敏感。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/auth/login",
		`{"username":"`+username+`","password":"`+password+`"}`, nil, "")
	_ = json.Unmarshal(rec.Body.Bytes(), &second)
	rec = do(t, router, http.MethodPost, "/api/v1/admin/auth/mfa/verify",
		`{"ticket":"`+second.Data.MFATicket+`","code":"`+enableResp.Data.RecoveryCodes[1]+`"}`, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("第三次登录应成功，实际 %d %s", rec.Code, rec.Body.String())
	}
	var finalCookie *http.Cookie
	var csrf2 string
	for _, c := range rec.Result().Cookies() {
		if c.Name == "mc_session" && c.Value != "" {
			finalCookie = c
		}
	}
	var verifyResp struct {
		Data struct {
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &verifyResp)
	csrf2 = verifyResp.Data.CSRFToken
	if finalCookie == nil || csrf2 == "" {
		t.Fatal("应返回会话与 CSRF token")
	}

	rec = do(t, router, http.MethodPost, "/api/v1/admin/auth/mfa/disable",
		`{"password":"wrong-password","code":"`+enableResp.Data.RecoveryCodes[2]+`"}`, finalCookie, csrf2)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("密码错误应拒绝解绑，实际 %d %s", rec.Code, rec.Body.String())
	}

	rec = do(t, router, http.MethodPost, "/api/v1/admin/auth/mfa/disable",
		`{"password":"`+password+`","code":"`+enableResp.Data.RecoveryCodes[2]+`"}`, finalCookie, csrf2)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("解绑应成功，实际 %d %s", rec.Code, rec.Body.String())
	}

	// 解绑后登录不再要求第二因子。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/auth/login",
		`{"username":"`+username+`","password":"`+password+`"}`, nil, "")
	var afterDisable struct {
		Data struct {
			MFARequired bool   `json:"mfa_required"`
			Username    string `json:"username"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &afterDisable); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if afterDisable.Data.MFARequired {
		t.Fatal("解绑后不应再要求第二因子")
	}
	if afterDisable.Data.Username != username {
		t.Fatalf("解绑后应直接登录成功，实际 %s", rec.Body.String())
	}

}

// loginWithPassword 用指定密码登录，返回会话 Cookie 与 CSRF token。
func loginWithPassword(t *testing.T, router http.Handler, username, password string) (*http.Cookie, string) {
	t.Helper()
	rec := do(t, router, http.MethodPost, "/api/v1/admin/auth/login",
		`{"username":"`+username+`","password":"`+password+`"}`, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("登录 %s 失败：%d %s", username, rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析登录响应失败: %v", err)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "mc_session" {
			return c, resp.Data.CSRFToken
		}
	}
	t.Fatal("登录未返回会话 Cookie")
	return nil, ""
}
