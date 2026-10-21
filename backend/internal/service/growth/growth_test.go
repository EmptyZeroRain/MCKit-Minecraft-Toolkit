package growth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"mckit/backend/internal/domain"
)

func testLevelResolver(maxLevel int) LevelResolver {
	return NewLevelResolver(
		&domain.GrowthLevelConfig{ID: 1, MaxLevel: maxLevel, StepXP: DefaultLevelStepXP},
		[]domain.GrowthLevelTitle{
			{MinLevel: 0, MaxLevel: 0, Title: "访客", Enabled: true},
			{MinLevel: 1, MaxLevel: 10, Title: "初来乍到", Enabled: true},
			{MinLevel: 5, MaxLevel: 5, Title: "五级称号", Enabled: true},
			{MinLevel: 100, MaxLevel: 100, Title: "百级传奇", Enabled: true},
		},
	)
}

// TestLevelThreshold 等级曲线：Lv.N 门槛 = step * N * (N-1) / 2。
func TestLevelThreshold(t *testing.T) {
	curve := LevelCurve{MaxLevel: 100, StepXP: 100}
	cases := map[int]int64{0: 0, 1: 0, 2: 100, 3: 300, 4: 600, 5: 1000, 10: 4500, 100: 495000}
	for level, want := range cases {
		if got := LevelThreshold(level, curve); got != want {
			t.Fatalf("Lv.%d 门槛 = %d，期望 %d", level, got, want)
		}
	}
}

// TestLevelForXP 经验反查等级边界。
func TestLevelForXP(t *testing.T) {
	curve := LevelCurve{MaxLevel: 100, StepXP: 100}
	cases := []struct {
		xp    int64
		level int
	}{
		{0, 1}, {99, 1}, {100, 2}, {299, 2}, {300, 3},
		{4499, 9}, {4500, 10}, {494999, 99}, {495000, 100}, {9999999, 100},
	}
	for _, tc := range cases {
		if got := levelForXP(tc.xp, curve); got != tc.level {
			t.Fatalf("%d XP 应为 Lv.%d，实际 Lv.%d", tc.xp, tc.level, got)
		}
	}
}

// TestLevelUnlimited 默认 0 上限表示不设等级上限。
func TestLevelUnlimited(t *testing.T) {
	curve := LevelCurve{MaxLevel: 0, StepXP: 100}
	const wantThreshold int64 = 100 * 10000 * 9999 / 2
	if got := LevelThreshold(10000, curve); got != wantThreshold {
		t.Fatalf("Lv.10000 门槛 = %d，期望 %d", got, wantThreshold)
	}
	if got := levelForXP(wantThreshold-1, curve); got != 9999 {
		t.Fatalf("%d XP 应为 Lv.9999，实际 Lv.%d", wantThreshold-1, got)
	}
	if got := levelForXP(wantThreshold, curve); got != 10000 {
		t.Fatalf("%d XP 应为 Lv.10000，实际 Lv.%d", wantThreshold, got)
	}

	resolver := testLevelResolver(0)
	view := resolver.View(wantThreshold, nil, GuestActiveDaysRequired)
	if view.Level != 10000 || view.IsMax {
		t.Fatalf("无限曲线下应为 Lv.10000 且非满级，实际 %+v", view)
	}
	if view.NextThreshold == nil || *view.NextThreshold <= wantThreshold {
		t.Fatalf("无限曲线下应继续展示下一级门槛，实际 %+v", view)
	}
}

// TestLevelView 覆盖等级计算的关键边界。
func TestLevelView(t *testing.T) {
	resolver := testLevelResolver(100)
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
		{"达到最高等级", 495000, 100, 0, true, 100},
		{"超过最高等级仍为满级", 9999999, 100, 0, true, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			view := resolver.View(tc.xp, nil, GuestActiveDaysRequired)
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

// TestLevelGuestGate 未满 7 个活跃日固定为 Lv.0，进度按活跃天数计算。
func TestLevelGuestGate(t *testing.T) {
	resolver := testLevelResolver(100)
	view := resolver.View(5000, nil, 3)
	if view.Level != 0 || view.Title != "访客" {
		t.Fatalf("3 个活跃日应停留 Lv.0，实际 %+v", view)
	}
	if view.NextThreshold != nil || view.XPToNext != 0 {
		t.Fatalf("访客阶段不应展示经验升级目标，实际 %+v", view)
	}
	if view.ActiveDays != 3 || view.GuestActiveDays != GuestActiveDaysRequired {
		t.Fatalf("活跃天数展示异常: %+v", view)
	}
	if view.GuestPendingXP != 5000 {
		t.Fatalf("访客阶段应暂存 5000 经验，实际 %d", view.GuestPendingXP)
	}
	wantPercent := 3 * 100 / GuestActiveDaysRequired
	if view.ProgressPercent != wantPercent {
		t.Fatalf("访客进度 = %d，期望 %d", view.ProgressPercent, wantPercent)
	}
	if got := resolver.View(5000, nil, GuestActiveDaysRequired); got.Level != 10 || got.GuestPendingXP != 0 {
		t.Fatalf("满 7 个活跃日后 5000 XP 应为 Lv.10 且不再暂存，实际 %+v", got)
	}
}

// TestLevelTitles 指定等级称号优先于区间称号，无规则时回退 Lv.N。
func TestLevelTitles(t *testing.T) {
	resolver := testLevelResolver(100)
	cases := map[int]string{
		0: "访客", 1: "初来乍到", 5: "五级称号", 10: "初来乍到",
		11: "Lv.11", 99: "Lv.99", 100: "百级传奇",
	}
	for level, want := range cases {
		if got := resolver.Title(level); got != want {
			t.Fatalf("Lv.%d 称号 = %q，期望 %q", level, got, want)
		}
	}
}

// TestBuildLevelTitle 称号规则校验：中文按字符计数，范围不能越界。
func TestBuildLevelTitle(t *testing.T) {
	ok, err := buildLevelTitle(LevelTitleInput{
		MinLevel: 1, MaxLevel: 10, Title: strings.Repeat("好", 100), Enabled: true,
	}, 9999)
	if err != nil || ok.Title == "" {
		t.Fatalf("100 个中文字符应合法: %+v err=%v", ok, err)
	}
	if _, err := buildLevelTitle(LevelTitleInput{
		MinLevel: 1, MaxLevel: 10, Title: strings.Repeat("好", 101), Enabled: true,
	}, 9999); !errors.Is(err, ErrLevelThreshold) {
		t.Fatalf("101 个中文字符应拒绝，实际 %v", err)
	}
	if _, err := buildLevelTitle(LevelTitleInput{
		MinLevel: 10, MaxLevel: 1, Title: "倒序", Enabled: true,
	}, 9999); !errors.Is(err, ErrLevelThreshold) {
		t.Fatalf("倒序区间应拒绝，实际 %v", err)
	}
	if _, err := buildLevelTitle(LevelTitleInput{
		MinLevel: 1, MaxLevel: 10000, Title: "越界", Enabled: true,
	}, 9999); !errors.Is(err, ErrLevelThreshold) {
		t.Fatalf("超过等级上限应拒绝，实际 %v", err)
	}
	if _, err := buildLevelTitle(LevelTitleInput{
		MinLevel: 100, MaxLevel: 100, Title: "超出当前上限但停用", Enabled: false,
	}, 50); err != nil {
		t.Fatalf("停用规则允许暂存于当前上限之外: %v", err)
	}
	if _, err := buildLevelTitle(LevelTitleInput{
		MinLevel: 10000, MaxLevel: 10000, Title: "无上限", Enabled: true,
	}, effectiveMaxLevel(LevelCurve{MaxLevel: 0})); err != nil {
		t.Fatalf("无限曲线应允许 Lv.10000 称号: %v", err)
	}
}

// TestLevelOverride 人工等级覆盖：覆盖值优先，经验变化不影响展示等级。
func TestLevelOverride(t *testing.T) {
	resolver := testLevelResolver(100)
	cases := []struct {
		name      string
		xp        int64
		override  *int
		active    int
		wantLevel int
	}{
		{"无覆盖按经验计算", 150, nil, 7, 2},
		{"覆盖值优先于经验", 150, intPtr(3), 7, 3},
		{"访客也可被人工覆盖", 0, intPtr(2), 1, 2},
		{"覆盖值不超过最高等级", 300, intPtr(999), 7, 100},
		{"覆盖值不低于最低等级", 0, intPtr(-1), 7, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			view := resolver.View(tc.xp, tc.override, tc.active)
			if view.Level != tc.wantLevel {
				t.Fatalf("等级 = %d，期望 %d", view.Level, tc.wantLevel)
			}
			if tc.override != nil && !view.Overridden {
				t.Fatal("人工覆盖时 Overridden 应为 true")
			}
			if tc.override != nil && view.NextThreshold != nil {
				t.Fatalf("人工覆盖时不应展示经验升级目标，实际 %+v", view)
			}
		})
	}
	if got := resolver.Level(0, intPtr(2), 7); got != 2 {
		t.Fatalf("0 XP + 覆盖 Lv.2 应为 Lv.2，实际 Lv.%d", got)
	}
}

func intPtr(v int) *int { return &v }

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
