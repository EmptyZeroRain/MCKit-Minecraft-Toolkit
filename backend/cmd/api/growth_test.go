package main

import (
	"fmt"
	"net/http"
	"testing"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/middleware"
)

// 用户成长系统集成测试（jingyan.md）。
//
// 覆盖端到端闭环：注册 / 登录 / 收藏等业务事件驱动经验发放，每日任务与
// 重复事件幂等，以及后台人工调整 / 冲正 / 权限边界。
// 需要 TEST_DATABASE_URL；未设置时整体跳过。

// itUserPass 集成测试普通用户密码。
const itUserPass = "growth-pass-123456"

// registerUser 注册一个普通用户并返回账号 ID。
func registerUser(t *testing.T, router http.Handler, username string) string {
	t.Helper()
	body := fmt.Sprintf(
		`{"username":%q,"nickname":"成长用户","email":%q,"password":%q,"captcha_id":"it","captcha_answer":"135790"}`,
		username, username+"@example.com", itUserPass)
	rec := do(t, router, http.MethodPost, "/api/v1/user/register", body, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("注册 %s 期望 200，实际 %d（%s）", username, rec.Code, rec.Body.String())
	}
	data := decodeData(t, rec)
	id, _ := data["id"].(string)
	if id == "" {
		t.Fatalf("注册响应缺少 id：%s", rec.Body.String())
	}
	return id
}

// loginUser 登录普通用户并返回会话 Cookie 与 CSRF token。
func loginUser(t *testing.T, router http.Handler, username string) (*http.Cookie, string) {
	t.Helper()
	rec := do(t, router, http.MethodPost, "/api/v1/user/login",
		fmt.Sprintf(`{"username":%q,"password":%q}`, username, itUserPass), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("登录 %s 期望 200，实际 %d（%s）", username, rec.Code, rec.Body.String())
	}
	data := decodeData(t, rec)
	csrf, _ := data["csrf_token"].(string)
	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == middleware.SessionCookieName() {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("登录响应缺少会话 Cookie")
	}
	return cookie, csrf
}

// growthOverview 读取本人成长概览。
func growthOverview(t *testing.T, router http.Handler, cookie *http.Cookie) (xp int64, level int, tasksCompleted int) {
	t.Helper()
	rec := do(t, router, http.MethodPost, "/api/v1/growth/overview", `{}`, cookie, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("成长概览期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	data := decodeData(t, rec)
	return toInt64(data["current_xp"]), toInt(data["level"]), toInt(data["today_tasks_completed"])
}

// TestGrowthEventDrivenAndIdempotent 覆盖业务事件驱动发奖与重复事件幂等。
func TestGrowthEventDrivenAndIdempotent(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	userID := registerUser(t, router, "growth-1")
	// 注册本身不发放经验（仅解锁「初次见面」成就，奖励为 0）。
	var g domain.UserGrowth
	if err := gdb.Where("user_id = ?", userID).First(&g).Error; err != nil {
		t.Fatalf("注册后应存在成长投影: %v", err)
	}

	cookie, csrf := loginUser(t, router, "growth-1")
	// 登录：daily_login 规则 5 XP + 每日登录任务 5 XP = 10 XP。
	xp, level, tasks := growthOverview(t, router, cookie)
	if xp != 10 || level != 1 || tasks != 1 {
		t.Fatalf("首次登录后期望 XP=10 Lv.1 任务完成 1，实际 XP=%d Lv.%d 完成=%d", xp, level, tasks)
	}

	// 同日再次登录：规则按业务日期幂等、任务已完成不再自增，经验不变。
	cookie, csrf = loginUser(t, router, "growth-1")
	xp, _, _ = growthOverview(t, router, cookie)
	if xp != 10 {
		t.Fatalf("同日重复登录不应重复发奖，期望 XP=10，实际 %d", xp)
	}

	// 收藏资源：resource_favorited 规则 2 XP + 收藏任务 2 XP = 14 XP。
	rec := do(t, router, http.MethodPost, "/api/v1/user/favorites/toggle",
		`{"kind":"resource","object_id":"res-1","label":"资源1","url":"/resources/res-1"}`, cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("收藏期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	xp, _, _ = growthOverview(t, router, cookie)
	if xp != 14 {
		t.Fatalf("首次收藏后期望 XP=14，实际 %d", xp)
	}

	// 取消收藏再重新收藏：同一对象不重复发奖。
	rec = do(t, router, http.MethodPost, "/api/v1/user/favorites/toggle",
		`{"kind":"resource","object_id":"res-1","label":"资源1","url":"/resources/res-1"}`, cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("取消收藏期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/user/favorites/toggle",
		`{"kind":"resource","object_id":"res-1","label":"资源1","url":"/resources/res-1"}`, cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("重新收藏期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	xp, _, _ = growthOverview(t, router, cookie)
	if xp != 14 {
		t.Fatalf("取消后重收藏不应重复发奖，期望 XP=14，实际 %d", xp)
	}

	// 收藏另一个对象：按目标对象各发一次。
	rec = do(t, router, http.MethodPost, "/api/v1/user/favorites/toggle",
		`{"kind":"resource","object_id":"res-2","label":"资源2","url":"/resources/res-2"}`, cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("收藏资源2 期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	xp, _, _ = growthOverview(t, router, cookie)
	if xp != 16 {
		t.Fatalf("收藏第二个资源后期望 XP=16，实际 %d", xp)
	}
}

// TestGrowthAdminAdjustReverseAndPermission 覆盖后台调整 / 冲正 / 权限边界。
func TestGrowthAdminAdjustReverseAndPermission(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	userID := registerUser(t, router, "growth-2")
	userCookie, _ := loginUser(t, router, "growth-2")
	// 登录后 10 XP。
	if xp, _, _ := growthOverview(t, router, userCookie); xp != 10 {
		t.Fatalf("登录后期望 XP=10，实际 %d", xp)
	}

	// 普通用户不能访问后台成长接口（无 ops.growth 权限）。
	rec := do(t, router, http.MethodPost, "/api/v1/admin/growth/overview", `{}`, userCookie, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("普通用户访问后台成长接口期望 403，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	// 未登录访问成长中心应 401。
	rec = do(t, router, http.MethodPost, "/api/v1/growth/overview", `{}`, nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未登录访问成长中心期望 401，实际 %d", rec.Code)
	}

	adminCookie, adminCSRF := loginAs(t, router, itAdminUser)

	// 人工调整 +100：XP 110，等级升到 Lv.2（阈值 100）。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/growth/adjust",
		fmt.Sprintf(`{"target_user_id":%q,"delta_xp":100,"reason":"测试补发","request_key":"it-adj-1"}`, userID), adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("人工调整期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	adjustLedgerID, _ := decodeData(t, rec)["id"].(string)
	if adjustLedgerID == "" {
		t.Fatal("人工调整响应缺少流水 ID")
	}
	if xp, level, _ := growthOverview(t, router, userCookie); xp != 110 || level != 2 {
		t.Fatalf("调整后期望 XP=110 Lv.2，实际 XP=%d Lv.%d", xp, level)
	}

	// 相同 request_key 重复提交：幂等，不重复调整。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/growth/adjust",
		fmt.Sprintf(`{"target_user_id":%q,"delta_xp":100,"reason":"测试补发","request_key":"it-adj-1"}`, userID), adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("重复调整期望 200（幂等），实际 %d（%s）", rec.Code, rec.Body.String())
	}
	if xp, _, _ := growthOverview(t, router, userCookie); xp != 110 {
		t.Fatalf("重复调整不应生效，期望 XP=110，实际 %d", xp)
	}

	// 冲正该笔调整：XP 回到 10，等级回落 Lv.1。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/growth/reverse",
		fmt.Sprintf(`{"id":%q,"reason":"测试冲正"}`, adjustLedgerID), adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("冲正期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	if xp, level, _ := growthOverview(t, router, userCookie); xp != 10 || level != 1 {
		t.Fatalf("冲正后期望 XP=10 Lv.1，实际 XP=%d Lv.%d", xp, level)
	}

	// 重复冲正同一流水：幂等，XP 不变。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/growth/reverse",
		fmt.Sprintf(`{"id":%q,"reason":"再次冲正"}`, adjustLedgerID), adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("重复冲正期望 200（幂等），实际 %d（%s）", rec.Code, rec.Body.String())
	}
	if xp, _, _ := growthOverview(t, router, userCookie); xp != 10 {
		t.Fatalf("重复冲正不应生效，期望 XP=10，实际 %d", xp)
	}

	// 冲正流水不可再次冲正：需先取到冲正流水 ID。
	var reversal domain.ExperienceLedger
	if err := gdb.Where("source_type = ?", domain.LedgerSourceReversal).First(&reversal).Error; err != nil {
		t.Fatalf("应存在冲正流水: %v", err)
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/growth/reverse",
		fmt.Sprintf(`{"id":%q,"reason":"非法二次冲正"}`, reversal.ID.String()), adminCookie, adminCSRF)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("冲正冲正流水期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 缺少原因的人工调整应被拒绝（422）。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/growth/adjust",
		fmt.Sprintf(`{"target_user_id":%q,"delta_xp":5}`, userID), adminCookie, adminCSRF)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("缺少原因期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 人工负向调整不会使余额为负（422）。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/growth/adjust",
		fmt.Sprintf(`{"target_user_id":%q,"delta_xp":-9999,"reason":"超额扣减"}`, userID), adminCookie, adminCSRF)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("超额扣减期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 后台概览与审计可读。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/growth/overview", `{}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("后台概览期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/growth/audit/list", `{}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("成长审计列表期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var auditCount int64
	if err := gdb.Model(&domain.GrowthAuditLog{}).Count(&auditCount).Error; err != nil {
		t.Fatalf("统计成长审计失败: %v", err)
	}
	if auditCount == 0 {
		t.Fatal("人工调整 / 冲正应写入成长审计日志")
	}
}

// TestGrowthAdminDefinitions 覆盖规则 / 任务 / 成就定义管理与历史补算。
func TestGrowthAdminDefinitions(t *testing.T) {
	_, router := integrationSetup(t)
	resetData(t, itDB)

	adminCookie, adminCSRF := loginAs(t, router, itAdminUser)

	// 新建经验规则。
	rec := do(t, router, http.MethodPost, "/api/v1/admin/growth/rules/create",
		`{"code":"it_bonus","name":"测试奖励","event_type":"tool.used_validly","xp_amount":7,"enabled":true}`,
		adminCookie, adminCSRF)
	if rec.Code != http.StatusCreated {
		t.Fatalf("新建规则期望 201，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	ruleID, _ := decodeData(t, rec)["id"].(string)

	// 重复编码应冲突。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/growth/rules/create",
		`{"code":"it_bonus","name":"重复","event_type":"tool.used_validly","xp_amount":1,"enabled":true}`,
		adminCookie, adminCSRF)
	if rec.Code != http.StatusConflict {
		t.Fatalf("重复规则编码期望 409，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 更新规则：版本号递增（版本化）。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/growth/rules/update",
		fmt.Sprintf(`{"id":%q,"code":"it_bonus","name":"测试奖励v2","event_type":"tool.used_validly","xp_amount":9,"enabled":true}`, ruleID),
		adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("更新规则期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	if v := toInt(decodeData(t, rec)["version"]); v != 2 {
		t.Fatalf("更新后规则版本期望 2，实际 %d", v)
	}

	// 非法事件类型应被拒绝。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/growth/rules/create",
		`{"code":"it_bad","name":"非法","event_type":"not.an.event","xp_amount":1,"enabled":true}`,
		adminCookie, adminCSRF)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("非法事件类型期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 删除未被引用的规则：成功。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/growth/rules/delete",
		fmt.Sprintf(`{"id":%q}`, ruleID), adminCookie, adminCSRF)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("删除未引用规则期望 204，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 每日任务定义列表可用。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/daily-tasks/list", `{}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("任务定义列表期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 成就定义列表可用。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/achievements/list", `{}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("成就定义列表期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 历史补算：对全部用户执行（当前无用户，返回 0）。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/achievements/recalculate", `{}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("成就补算期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 等级阈值非法（Lv.1 必须 0 XP）应被拒绝。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/growth/levels/update",
		`{"level":1,"threshold_xp":50,"title":"Lv.1","enabled":true}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("非法等级阈值期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}
}

// toInt64 将 JSON 数字（float64 / int64 / json.Number）安全转 int64。
func toInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	default:
		return 0
	}
}

// toInt 将 JSON 数字安全转 int。
func toInt(v any) int { return int(toInt64(v)) }
