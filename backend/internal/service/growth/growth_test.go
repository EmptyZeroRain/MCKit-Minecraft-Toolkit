package growth

import (
	"testing"
	"time"

	"mckit/backend/internal/domain"
)

// TestComputeLevel 覆盖等级计算的关键边界（jingyan.md §5）。
func TestComputeLevel(t *testing.T) {
	levels := []domain.GrowthLevel{
		{Level: 1, ThresholdXP: 0, Title: "初来乍到", Enabled: true},
		{Level: 2, ThresholdXP: 100, Title: "方块学徒", Enabled: true},
		{Level: 3, ThresholdXP: 300, Title: "探索者", Enabled: true},
	}
	cases := []struct {
		name        string
		xp          int64
		wantLevel   int
		wantToNext  int64
		wantIsMax   bool
		wantPercent int
	}{
		{"零经验为 Lv.1", 0, 1, 100, false, 0},
		{"刚好达标升级", 100, 2, 200, false, 0},
		{"区间中间进度", 200, 2, 100, false, 50},
		{"达到最高等级", 300, 3, 0, true, 100},
		{"超过最高等级仍为满级", 9999, 3, 0, true, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			view := computeLevel(levels, tc.xp)
			if view.Level != tc.wantLevel {
				t.Fatalf("等级 = %d，期望 %d", view.Level, tc.wantLevel)
			}
			if view.XPToNext != tc.wantToNext {
				t.Fatalf("距下一级 = %d，期望 %d", view.XPToNext, tc.wantToNext)
			}
			if view.IsMax != tc.wantIsMax {
				t.Fatalf("满级标记 = %v，期望 %v", view.IsMax, tc.wantIsMax)
			}
			if view.ProgressPercent != tc.wantPercent {
				t.Fatalf("进度百分比 = %d，期望 %d", view.ProgressPercent, tc.wantPercent)
			}
		})
	}
}

// TestComputeLevelIgnoresDisabled 停用等级不参与计算。
func TestComputeLevelIgnoresDisabled(t *testing.T) {
	levels := []domain.GrowthLevel{
		{Level: 1, ThresholdXP: 0, Title: "Lv.1", Enabled: true},
		{Level: 2, ThresholdXP: 100, Title: "Lv.2", Enabled: false},
		{Level: 3, ThresholdXP: 300, Title: "Lv.3", Enabled: true},
	}
	if got := computeLevel(levels, 150).Level; got != 1 {
		t.Fatalf("停用 Lv.2 后 150 XP 应为 Lv.1，实际 Lv.%d", got)
	}
	if got := computeLevel(levels, 300).Level; got != 3 {
		t.Fatalf("300 XP 应为 Lv.3，实际 Lv.%d", got)
	}
}

// TestComputeLevelEmptyThresholds 阈值表缺失时兜底 Lv.1。
func TestComputeLevelEmptyThresholds(t *testing.T) {
	view := computeLevel(nil, 12345)
	if view.Level != 1 || !view.IsMax {
		t.Fatalf("空阈值表应兜底 Lv.1 满级，实际 %+v", view)
	}
}

// TestBusinessDate 业务日期按站点时区换算。
func TestBusinessDate(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skipf("缺少时区数据: %v", err)
	}
	// UTC 2026-01-01 23:30 在东八区已是 2026-01-02。
	utc := time.Date(2026, 1, 1, 23, 30, 0, 0, time.UTC)
	if got := businessDate(utc, loc); got != "2026-01-02" {
		t.Fatalf("业务日期 = %s，期望 2026-01-02", got)
	}
	if got := businessDate(utc, time.UTC); got != "2026-01-01" {
		t.Fatalf("UTC 业务日期 = %s，期望 2026-01-01", got)
	}
}

// TestDayRange 业务日期区间为 [start, end) 且覆盖 24 小时。
func TestDayRange(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skipf("缺少时区数据: %v", err)
	}
	start, end, err := dayRange("2026-01-02", loc)
	if err != nil {
		t.Fatalf("dayRange 失败: %v", err)
	}
	if end.Sub(start) != 24*time.Hour {
		t.Fatalf("区间长度 = %v，期望 24h", end.Sub(start))
	}
	if businessDate(start, loc) != "2026-01-02" {
		t.Fatalf("区间起点业务日期错误: %s", businessDate(start, loc))
	}
	if _, _, err := dayRange("not-a-date", loc); err == nil {
		t.Fatal("非法日期应返回错误")
	}
}

// TestNormalizeTimezone 非法 / 空时区回退默认时区。
func TestNormalizeTimezone(t *testing.T) {
	if got := normalizeTimezone("").String(); got != DefaultTimezone {
		t.Fatalf("空时区应回退 %s，实际 %s", DefaultTimezone, got)
	}
	if got := normalizeTimezone("Not/AZone").String(); got != DefaultTimezone {
		t.Fatalf("非法时区应回退 %s，实际 %s", DefaultTimezone, got)
	}
	if got := normalizeTimezone("UTC").String(); got != "UTC" {
		t.Fatalf("合法时区应保留，实际 %s", got)
	}
}

// TestIdempotencyKeys 幂等键生成策略（jingyan.md §7.1）。
func TestIdempotencyKeys(t *testing.T) {
	if got := ruleKeyForSubject("like", "u1", "p1"); got != "rule:like:u1:subject:p1" {
		t.Fatalf("单对象键错误: %s", got)
	}
	if got := ruleKeyForDay("login", "u1", "2026-01-02"); got != "rule:login:u1:day:2026-01-02" {
		t.Fatalf("每日键错误: %s", got)
	}
	if got := ruleKeyForEvent("reg", "u1", "e1"); got != "rule:reg:u1:event:e1" {
		t.Fatalf("事件键错误: %s", got)
	}
	if got := taskRewardKey("daily_login", "u1", "2026-01-02"); got != "task:daily_login:u1:2026-01-02" {
		t.Fatalf("任务键错误: %s", got)
	}
	if got := achievementRewardKey("level_3", "u1"); got != "achievement:level_3:u1" {
		t.Fatalf("成就键错误: %s", got)
	}
	if got := manualAdjustKey("u1", "req1"); got != "manual:u1:req1" {
		t.Fatalf("人工调整键错误: %s", got)
	}
	if got := reversalKey("ledger1"); got != "reversal:ledger1" {
		t.Fatalf("冲正键错误: %s", got)
	}
}

// TestValidateIconURL 图标地址只允许 https:// 或站内 / 路径。
func TestValidateIconURL(t *testing.T) {
	cases := map[string]bool{
		"":                              true,
		"/icons/a.png":                  true,
		"https://cdn.example.com/a.png": true,
		"HTTPS://CDN.EXAMPLE.COM/a.png": true,
		"http://cdn.example.com/a.png":  false,
		"//evil.com/a.png":              false,
		"javascript:alert(1)":           false,
		"data:image/png;base64,AAAA":    false,
	}
	for url, want := range cases {
		err := validateIconURL(url)
		if want && err != nil {
			t.Fatalf("图标地址 %q 应合法，实际 %v", url, err)
		}
		if !want && err == nil {
			t.Fatalf("图标地址 %q 应非法", url)
		}
	}
}

// TestValidateAchievementInput 成就条件与事件类型校验。
func TestValidateAchievementInput(t *testing.T) {
	base := AchievementInput{
		Name: "测试成就", Category: "探索", ConditionType: domain.AchievementCondEventCount,
		ConditionEventType: domain.EventUserRegistered, TargetValue: 1, Enabled: true,
	}
	if err := validateAchievementInput(base); err != nil {
		t.Fatalf("合法成就应通过: %v", err)
	}
	bad := base
	bad.ConditionType = "sql_injection"
	if err := validateAchievementInput(bad); err != ErrBadCondition {
		t.Fatalf("非法条件类型应返回 ErrBadCondition，实际 %v", err)
	}
	bad = base
	bad.ConditionEventType = "unknown.event"
	if err := validateAchievementInput(bad); err != ErrInvalidEvent {
		t.Fatalf("非法事件类型应返回 ErrInvalidEvent，实际 %v", err)
	}
	// level_reached / daily_task_days 不需要事件类型。
	level := base
	level.ConditionType = domain.AchievementCondLevelReached
	level.ConditionEventType = ""
	level.TargetValue = 3
	if err := validateAchievementInput(level); err != nil {
		t.Fatalf("等级成就应通过: %v", err)
	}
}

// TestNormalizeRuleCode 编码字符集校验。
func TestNormalizeRuleCode(t *testing.T) {
	if _, err := normalizeRuleCode("daily_login"); err != nil {
		t.Fatalf("合法编码应通过: %v", err)
	}
	for _, bad := range []string{"", "带中文", "has space", "a/b", "a;drop"} {
		if _, err := normalizeRuleCode(bad); err == nil {
			t.Fatalf("非法编码 %q 应被拒绝", bad)
		}
	}
}

// TestParsePositiveInt 空 / 非法 / 非正数返回 nil（表示不限制）。
func TestParsePositiveInt(t *testing.T) {
	if got := parsePositiveInt(""); got != nil {
		t.Fatalf("空串应为 nil，实际 %v", *got)
	}
	if got := parsePositiveInt("0"); got != nil {
		t.Fatalf("0 应为 nil，实际 %v", *got)
	}
	if got := parsePositiveInt("-3"); got != nil {
		t.Fatalf("负数应为 nil，实际 %v", *got)
	}
	if got := parsePositiveInt("5"); got == nil || *got != 5 {
		t.Fatalf("5 应解析为 5，实际 %v", got)
	}
}

// TestDomainEventTypes 事件类型与成就条件允许列表自洽。
func TestDomainEventTypes(t *testing.T) {
	if !domain.IsValidGrowthEventType(domain.EventUserRegistered) {
		t.Fatal("user.registered 应为合法事件类型")
	}
	if domain.IsValidGrowthEventType("bogus") {
		t.Fatal("未知事件类型不应合法")
	}
	for _, c := range domain.AchievementConditionTypes {
		if !domain.IsValidAchievementCondition(c) {
			t.Fatalf("成就条件 %s 应合法", c)
		}
	}
}
