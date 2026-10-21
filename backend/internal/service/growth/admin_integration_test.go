package growth

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mckit/backend/internal/accountid"
	"mckit/backend/internal/domain"
	db "mckit/backend/internal/repository"
)

const growthAdminSchema = "growth_admin_it"

func withGrowthSearchPath(dsn, schema string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + schema
}

func setupGrowthAdminDB(t *testing.T) *gorm.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("未设置 TEST_DATABASE_URL，跳过数据库集成测试")
	}
	ctx := context.Background()
	adminDB, err := db.Open(url)
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	if err := adminDB.Exec("DROP SCHEMA IF EXISTS " + growthAdminSchema + " CASCADE").Error; err != nil {
		t.Fatalf("清理测试 schema 失败: %v", err)
	}
	if err := adminDB.Exec("CREATE SCHEMA " + growthAdminSchema).Error; err != nil {
		t.Fatalf("创建测试 schema 失败: %v", err)
	}
	if sqlDB, err := adminDB.DB(); err == nil {
		_ = sqlDB.Close()
	}
	gdb, err := db.Open(withGrowthSearchPath(url, growthAdminSchema))
	if err != nil {
		t.Fatalf("连接测试 schema 失败: %v", err)
	}
	if _, err := db.Migrate(ctx, gdb, "../../../migrations"); err != nil {
		t.Fatalf("执行迁移失败: %v", err)
	}
	return gdb
}

func createGrowthUser(t *testing.T, gdb *gorm.DB, username, role string) *domain.User {
	t.Helper()
	userNo, err := accountid.Generate()
	if err != nil {
		t.Fatalf("生成 user_no 失败: %v", err)
	}
	u := &domain.User{Username: username, UserNo: userNo, Role: role, IsActive: true}
	if err := gdb.Create(u).Error; err != nil {
		t.Fatalf("创建测试账号失败: %v", err)
	}
	return u
}

// TestGrowthAdminControlsIntegration 覆盖单用户等级人工覆盖、成就授予 / 撤销与奖励经验幂等。
func TestGrowthAdminControlsIntegration(t *testing.T) {
	gdb := setupGrowthAdminDB(t)
	ctx := context.Background()
	svc := NewService(gdb, "UTC")

	user := createGrowthUser(t, gdb, "it-growth-user", domain.RoleUser)
	operator := createGrowthUser(t, gdb, "it-growth-admin", domain.RoleAdmin)

	// 新用户默认 Lv.0；直接覆盖到 Lv.3，不再使用“基础等级 + 偏移”。
	result, err := svc.AdjustLevel(ctx, AdjustLevelInput{
		TargetUserID: user.UserNo,
		TargetLevel:  3,
		Reason:       "集成测试升级",
		OperatorID:   operator.ID.String(),
	})
	if err != nil {
		t.Fatalf("等级调整失败: %v", err)
	}
	if result.Level != 3 || result.BaseLevel != 0 || !result.Overridden {
		t.Fatalf("等级调整结果异常: %+v", result)
	}
	var g domain.UserGrowth
	if err := gdb.Where("user_id = ?", user.ID).First(&g).Error; err != nil {
		t.Fatalf("读取成长投影失败: %v", err)
	}
	if g.Level != 3 || g.LevelOverride == nil || *g.LevelOverride != 3 {
		t.Fatalf("成长投影未持久化覆盖等级: level=%d override=%v", g.Level, g.LevelOverride)
	}

	// 再覆盖为 Lv.2。
	result, err = svc.AdjustLevel(ctx, AdjustLevelInput{
		TargetUserID: user.UserNo,
		TargetLevel:  2,
		Reason:       "集成测试降级",
		OperatorID:   operator.ID.String(),
	})
	if err != nil || result.Level != 2 || !result.Overridden {
		t.Fatalf("二次等级调整异常: result=%+v err=%v", result, err)
	}

	// 覆盖期间经验继续累积，但展示等级不会被自动重算改回。
	xpUser := createGrowthUser(t, gdb, "it-growth-xp-override", domain.RoleUser)
	if _, err := svc.AdjustLevel(ctx, AdjustLevelInput{
		TargetUserID: xpUser.UserNo,
		TargetLevel:  5,
		Reason:       "覆盖稳定性测试",
		OperatorID:   operator.ID.String(),
	}); err != nil {
		t.Fatalf("覆盖稳定性测试等级调整失败: %v", err)
	}
	if _, err := svc.Adjust(ctx, AdjustInput{
		TargetUserID: xpUser.UserNo,
		DeltaXP:      5000,
		Reason:       "覆盖期间经验累积",
		OperatorID:   operator.ID.String(),
	}); err != nil {
		t.Fatalf("覆盖期间调整经验失败: %v", err)
	}
	var xpGrowth domain.UserGrowth
	if err := gdb.Where("user_id = ?", xpUser.ID).First(&xpGrowth).Error; err != nil {
		t.Fatalf("读取成长投影失败: %v", err)
	}
	if xpGrowth.CurrentXP != 5000 || xpGrowth.Level != 5 {
		t.Fatalf("覆盖期间经验应累积且等级保持 Lv.5，实际 xp=%d level=%d", xpGrowth.CurrentXP, xpGrowth.Level)
	}

	// 超出等级范围应拒绝，且不写入。
	if _, err := svc.AdjustLevel(ctx, AdjustLevelInput{
		TargetUserID: user.UserNo,
		TargetLevel:  HardMaxLevel + 1,
		Reason:       "越界测试",
		OperatorID:   operator.ID.String(),
	}); !errors.Is(err, ErrLevelRange) {
		t.Fatalf("越界等级应返回 ErrLevelRange，实际 %v", err)
	}

	// 该表单只接受对外 ID：内部 UUID 必须被明确拒绝。
	if _, err := svc.AdjustLevel(ctx, AdjustLevelInput{
		TargetUserID: user.ID.String(),
		TargetLevel:  1,
		Reason:       "UUID 输入测试",
		OperatorID:   operator.ID.String(),
	}); !errors.Is(err, ErrLevelAdjustUserIDRequired) {
		t.Fatalf("内部 UUID 应返回 ErrLevelAdjustUserIDRequired，实际 %v", err)
	}

	// 后台账号同样有对外 ID，且统一账号可拥有成长投影；大小写不敏感。
	adminTarget := createGrowthUser(t, gdb, "it-growth-target-admin", domain.RoleAdmin)
	adminResult, err := svc.AdjustLevel(ctx, AdjustLevelInput{
		TargetUserID: strings.ToLower(adminTarget.UserNo),
		TargetLevel:  2,
		Reason:       "管理员等级调整测试",
		OperatorID:   operator.ID.String(),
	})
	if err != nil {
		t.Fatalf("管理员对外 ID 等级调整失败: %v", err)
	}
	if adminResult.UserID != adminTarget.ID.String() || adminResult.Level != 2 || !adminResult.Overridden {
		t.Fatalf("管理员等级调整结果异常: %+v", adminResult)
	}

	// 用户名不是本表单的合法标识，应按对外 ID 未找到处理。
	if _, err := svc.AdjustLevel(ctx, AdjustLevelInput{
		TargetUserID: user.Username,
		TargetLevel:  1,
		Reason:       "用户名输入测试",
		OperatorID:   operator.ID.String(),
	}); !errors.Is(err, ErrLevelAdjustUserNotFound) {
		t.Fatalf("用户名应返回 ErrLevelAdjustUserNotFound，实际 %v", err)
	}

	// 创建一个奖励经验成就，手动授予后应发一次经验。
	achievement := &domain.AchievementDefinition{
		Code:          "it_manual_grant",
		Name:          "集成测试成就",
		Description:   "手动授予测试",
		Category:      "测试",
		Rarity:        "common",
		ConditionType: domain.AchievementCondEventCount,
		TargetValue:   1,
		RewardXP:      20,
		Enabled:       true,
		SortOrder:     999,
	}
	if err := gdb.Create(achievement).Error; err != nil {
		t.Fatalf("创建成就定义失败: %v", err)
	}

	ua, err := svc.GrantAchievement(ctx, AchievementGrantInput{
		TargetUserID:  user.ID.String(),
		AchievementID: achievement.ID.String(),
		Reason:        "集成测试授予",
		OperatorID:    operator.ID.String(),
	})
	if err != nil {
		t.Fatalf("授予成就失败: %v", err)
	}
	if ua.RevokedAt != nil || ua.GrantSource != "manual" {
		t.Fatalf("授予结果异常: %+v", ua)
	}
	if err := gdb.Where("user_id = ?", user.ID).First(&g).Error; err != nil {
		t.Fatalf("读取成长投影失败: %v", err)
	}
	if g.CurrentXP != 20 {
		t.Fatalf("手动授予应发放 20 经验，实际 %d", g.CurrentXP)
	}

	// 重复授予应冲突。
	if _, err := svc.GrantAchievement(ctx, AchievementGrantInput{
		TargetUserID: user.ID.String(), AchievementID: achievement.ID.String(),
		Reason: "重复授予", OperatorID: operator.ID.String(),
	}); !errors.Is(err, ErrAchievementAlreadyUnlocked) {
		t.Fatalf("重复授予应返回 ErrAchievementAlreadyUnlocked，实际 %v", err)
	}

	// 撤销后自动可见列表应排除，后台列表标记 revoked。
	if _, err := svc.RevokeAchievement(ctx, AchievementRevokeInput{
		TargetUserID: user.ID.String(), AchievementID: achievement.ID.String(),
		Reason: "集成测试撤销", OperatorID: operator.ID.String(),
	}); err != nil {
		t.Fatalf("撤销成就失败: %v", err)
	}
	active, err := svc.repo.ListUserAchievements(ctx, user.ID.String())
	if err != nil {
		t.Fatalf("查询已解锁成就失败: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("撤销后不应出现在已解锁列表，实际 %d 条", len(active))
	}
	adminRows, err := svc.ListAdminUserAchievements(ctx, user.ID.String())
	if err != nil {
		t.Fatalf("后台查询用户成就失败: %v", err)
	}
	found := false
	for _, row := range adminRows {
		if row.Code == achievement.Code {
			found = true
			if !row.Revoked || row.Unlocked {
				t.Fatalf("后台撤销状态异常: %+v", row)
			}
		}
	}
	if !found {
		t.Fatal("后台用户成就列表缺少测试成就")
	}

	// 重新授予不重复发奖励经验。
	if _, err := svc.GrantAchievement(ctx, AchievementGrantInput{
		TargetUserID: user.ID.String(), AchievementID: achievement.ID.String(),
		Reason: "集成测试重新授予", OperatorID: operator.ID.String(),
	}); err != nil {
		t.Fatalf("重新授予失败: %v", err)
	}
	if err := gdb.Where("user_id = ?", user.ID).First(&g).Error; err != nil {
		t.Fatalf("读取成长投影失败: %v", err)
	}
	if g.CurrentXP != 20 {
		t.Fatalf("重新授予不应重复发经验，实际 %d", g.CurrentXP)
	}

	// 空原因拒绝。
	if _, err := svc.RevokeAchievement(ctx, AchievementRevokeInput{
		TargetUserID: user.ID.String(), AchievementID: achievement.ID.String(),
		Reason: "  ", OperatorID: operator.ID.String(),
	}); !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("空原因应返回 ErrReasonRequired，实际 %v", err)
	}
}

// TestGrowthLevelManagementIntegration 覆盖等级曲线、称号规则、用户等级重算与人工覆盖。
// TestRefreshActiveDaysIntegration 验证活跃日按业务时区去重、满 7 天后封顶。
func TestRefreshActiveDaysIntegration(t *testing.T) {
	gdb := setupGrowthAdminDB(t)
	ctx := context.Background()
	svc := NewService(gdb, "Asia/Shanghai")
	user := createGrowthUser(t, gdb, "it-active-days", domain.RoleUser)

	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("加载业务时区失败: %v", err)
	}
	// 每天 23:30（业务时区）产生一个登录事件，连续 7 个自然日。
	start := time.Date(2026, 10, 1, 23, 30, 0, 0, loc)
	for i := 0; i < GuestActiveDaysRequired; i++ {
		err := svc.HandleEvent(ctx, domain.GrowthEvent{
			EventID:      uuid.New(),
			EventType:    domain.EventUserLoginSucceeded,
			ActorUserID:  user.ID,
			SourceModule: "it",
			OccurredAt:   start.AddDate(0, 0, i),
		})
		if err != nil {
			t.Fatalf("处理第 %d 个活跃日事件失败: %v", i+1, err)
		}
	}

	var g domain.UserGrowth
	if err := gdb.Where("user_id = ?", user.ID).First(&g).Error; err != nil {
		t.Fatalf("读取成长投影失败: %v", err)
	}
	if g.ActiveDays != GuestActiveDaysRequired {
		t.Fatalf("7 个不同自然日后活跃天数应为 %d，实际 %d", GuestActiveDaysRequired, g.ActiveDays)
	}
	if g.Level < 1 {
		t.Fatalf("满 7 个活跃日后应进入经验等级体系，实际 Lv.%d", g.Level)
	}

	// 第 8 个活跃日不再增长，缓存封顶为 7。
	if err := svc.HandleEvent(ctx, domain.GrowthEvent{
		EventID:      uuid.New(),
		EventType:    domain.EventUserLoginSucceeded,
		ActorUserID:  user.ID,
		SourceModule: "it",
		OccurredAt:   start.AddDate(0, 0, GuestActiveDaysRequired),
	}); err != nil {
		t.Fatalf("处理第 8 个活跃日事件失败: %v", err)
	}
	g = domain.UserGrowth{}
	if err := gdb.Where("user_id = ?", user.ID).First(&g).Error; err != nil {
		t.Fatalf("读取成长投影失败: %v", err)
	}
	if g.ActiveDays != GuestActiveDaysRequired {
		t.Fatalf("活跃天数应封顶为 %d，实际 %d", GuestActiveDaysRequired, g.ActiveDays)
	}
}

func TestGrowthLevelManagementIntegration(t *testing.T) {
	gdb := setupGrowthAdminDB(t)
	ctx := context.Background()
	svc := NewService(gdb, "UTC")

	user := createGrowthUser(t, gdb, "it-level-manage-user", domain.RoleUser)
	operator := createGrowthUser(t, gdb, "it-level-manage-admin", domain.RoleAdmin)

	cfg, err := svc.GetLevelConfig(ctx)
	if err != nil {
		t.Fatalf("读取等级曲线失败: %v", err)
	}
	if cfg.MaxLevel != DefaultMaxLevel || cfg.StepXP != DefaultLevelStepXP {
		t.Fatalf("默认等级曲线异常: %+v", cfg)
	}
	titles, err := svc.ListLevelTitles(ctx)
	if err != nil {
		t.Fatalf("读取称号规则失败: %v", err)
	}
	if len(titles) != 3 {
		t.Fatalf("默认称号规则应为 3 条（Lv.0 / Lv.1-10 / Lv.100），实际 %d: %+v", len(titles), titles)
	}

	if err := gdb.Create(&domain.UserGrowth{
		UserID: user.ID, CurrentXP: 7000, Level: 12, ActiveDays: GuestActiveDaysRequired,
	}).Error; err != nil {
		t.Fatalf("创建成长投影失败: %v", err)
	}

	// Lv.1-10 为区间「初来乍到」，Lv.100 为指定称号。
	resolver, err := svc.levelResolver(ctx)
	if err != nil {
		t.Fatalf("读取等级解析器失败: %v", err)
	}
	if got := resolver.Title(1); got != "初来乍到" {
		t.Fatalf("Lv.1 称号 = %q", got)
	}
	if got := resolver.Title(10); got != "初来乍到" {
		t.Fatalf("Lv.10 称号 = %q", got)
	}
	if got := resolver.Title(100); got != "MCkit 传奇" {
		t.Fatalf("Lv.100 称号 = %q", got)
	}
	if got := resolver.Title(11); got != "Lv.11" {
		t.Fatalf("无规则等级应回退 Lv.N，实际 %q", got)
	}

	// 区间称号：20-30 可创建，重叠区间拒绝；指定等级 20 覆盖区间。
	if _, err := svc.CreateLevelTitle(ctx, LevelTitleInput{
		MinLevel: 20, MaxLevel: 30, Title: "探索者", Enabled: true,
	}, operator.ID.String()); err != nil {
		t.Fatalf("新增区间称号失败: %v", err)
	}
	if _, err := svc.CreateLevelTitle(ctx, LevelTitleInput{
		MinLevel: 25, MaxLevel: 35, Title: "重叠区间", Enabled: true,
	}, operator.ID.String()); !errors.Is(err, ErrLevelExists) {
		t.Fatalf("重叠区间应返回 ErrLevelExists，实际 %v", err)
	}
	exact, err := svc.CreateLevelTitle(ctx, LevelTitleInput{
		MinLevel: 20, MaxLevel: 20, Title: "二十级", Enabled: true,
	}, operator.ID.String())
	if err != nil {
		t.Fatalf("新增指定等级称号失败: %v", err)
	}
	if _, err := svc.CreateLevelTitle(ctx, LevelTitleInput{
		MinLevel: 20, MaxLevel: 20, Title: "重复指定等级", Enabled: true,
	}, operator.ID.String()); !errors.Is(err, ErrLevelExists) {
		t.Fatalf("重复指定等级称号应返回 ErrLevelExists，实际 %v", err)
	}
	resolver, err = svc.levelResolver(ctx)
	if err != nil {
		t.Fatalf("读取等级解析器失败: %v", err)
	}
	if got := resolver.Title(20); got != "二十级" {
		t.Fatalf("指定等级称号应覆盖区间，实际 %q", got)
	}
	if got := resolver.Title(21); got != "探索者" {
		t.Fatalf("区间内其他等级称号 = %q", got)
	}
	if _, err := svc.UpdateLevelTitle(ctx, exact.ID.String(), LevelTitleInput{
		MinLevel: 20, MaxLevel: 20, Title: "二十级·改", Enabled: true,
	}, operator.ID.String()); err != nil {
		t.Fatalf("更新指定等级称号失败: %v", err)
	}

	// 调整曲线后自动重算：step 200 下 7000 XP 为 Lv.8。
	fixed, err := svc.UpdateLevelConfig(ctx, LevelConfigInput{MaxLevel: 5000, StepXP: 200}, operator.ID.String())
	if err != nil {
		t.Fatalf("更新等级曲线失败: %v", err)
	}
	if fixed != 1 {
		t.Fatalf("应重算 1 个用户，实际 %d", fixed)
	}
	var g domain.UserGrowth
	if err := gdb.Where("user_id = ?", user.ID).First(&g).Error; err != nil {
		t.Fatalf("读取成长投影失败: %v", err)
	}
	if g.Level != 8 {
		t.Fatalf("step=200 时 7000 XP 应为 Lv.8，实际 Lv.%d", g.Level)
	}

	// 单用户覆盖：Lv.500 显示指定称号，经验不变；超出上限拒绝。
	adjust, err := svc.AdjustLevel(ctx, AdjustLevelInput{
		TargetUserID: user.UserNo, TargetLevel: 500, Reason: "集成测试覆盖",
		OperatorID: operator.ID.String(),
	})
	if err != nil {
		t.Fatalf("调整等级失败: %v", err)
	}
	if adjust.Level != 500 || adjust.BaseLevel != 8 || !adjust.Overridden {
		t.Fatalf("等级覆盖结果异常: %+v", adjust)
	}
	if adjust.LevelTitle != "Lv.500" {
		t.Fatalf("未配置称号时应回退 Lv.500，实际 %q", adjust.LevelTitle)
	}
	if _, err := svc.AdjustLevel(ctx, AdjustLevelInput{
		TargetUserID: user.UserNo, TargetLevel: 5001, Reason: "超限",
		OperatorID: operator.ID.String(),
	}); !errors.Is(err, ErrLevelRange) {
		t.Fatalf("超出等级上限应返回 ErrLevelRange，实际 %v", err)
	}
	if err := gdb.Where("user_id = ?", user.ID).First(&g).Error; err != nil {
		t.Fatalf("读取成长投影失败: %v", err)
	}
	if g.Level != 500 || g.LevelOverride == nil || *g.LevelOverride != 500 {
		t.Fatalf("人工覆盖未落库: level=%d override=%v", g.Level, g.LevelOverride)
	}

	// 降低上限后覆盖值同步收敛到新上限。
	if _, err := svc.UpdateLevelConfig(ctx, LevelConfigInput{MaxLevel: 400, StepXP: 200}, operator.ID.String()); err != nil {
		t.Fatalf("降低等级上限失败: %v", err)
	}
	if err := gdb.Where("user_id = ?", user.ID).First(&g).Error; err != nil {
		t.Fatalf("读取成长投影失败: %v", err)
	}
	if g.Level != 400 || g.LevelOverride == nil || *g.LevelOverride != 400 {
		t.Fatalf("降低上限后覆盖值应收敛到 400: level=%d override=%v", g.Level, g.LevelOverride)
	}

	// 删除称号规则后对应等级回退 Lv.N。
	if err := svc.DeleteLevelTitle(ctx, exact.ID.String(), operator.ID.String()); err != nil {
		t.Fatalf("删除指定等级称号失败: %v", err)
	}
	resolver, err = svc.levelResolver(ctx)
	if err != nil {
		t.Fatalf("读取等级解析器失败: %v", err)
	}
	if got := resolver.Title(20); got != "探索者" {
		t.Fatalf("删除指定等级称号后应回退区间称号，实际 %q", got)
	}
}

// TestAdminGrowthUserDetailIntegration 覆盖后台用户详情聚合：等级 / 成就 / 经验 / 礼物 / 审计。
func TestAdminGrowthUserDetailIntegration(t *testing.T) {
	gdb := setupGrowthAdminDB(t)
	ctx := context.Background()
	svc := NewService(gdb, "UTC")

	user := createGrowthUser(t, gdb, "it-growth-detail-user", domain.RoleUser)
	operator := createGrowthUser(t, gdb, "it-growth-detail-admin", domain.RoleAdmin)

	if _, err := svc.Adjust(ctx, AdjustInput{
		TargetUserID: user.UserNo,
		DeltaXP:      30,
		Reason:       "详情测试补发",
		OperatorID:   operator.ID.String(),
	}); err != nil {
		t.Fatalf("调整经验失败: %v", err)
	}

	achievement := &domain.AchievementDefinition{
		Code:          "it_detail_achievement",
		Name:          "详情测试成就",
		Description:   "用于详情聚合测试",
		Category:      "测试",
		Rarity:        "rare",
		ConditionType: domain.AchievementCondEventCount,
		TargetValue:   1,
		RewardXP:      10,
		Enabled:       true,
		SortOrder:     1000,
	}
	if err := gdb.Create(achievement).Error; err != nil {
		t.Fatalf("创建成就失败: %v", err)
	}
	if _, err := svc.GrantAchievement(ctx, AchievementGrantInput{
		TargetUserID:  user.UserNo,
		AchievementID: achievement.ID.String(),
		Reason:        "详情测试授予",
		OperatorID:    operator.ID.String(),
	}); err != nil {
		t.Fatalf("授予成就失败: %v", err)
	}

	if err := gdb.Create(&domain.GiftWallet{
		UserID: user.ID, GeneralBalance: 12, CheckinBalance: 0,
	}).Error; err != nil {
		t.Fatalf("创建礼物钱包失败: %v", err)
	}
	if err := gdb.Create(&domain.GiftLedger{
		UserID: user.ID, Amount: 12, CoinType: domain.GiftCoinGeneral, CoinBalanceAfter: 12,
		EventType: domain.GiftEventAdminAdjustment, SourceType: "admin_action",
		IdempotencyKey: "it-detail-gift", Reason: "活动奖励",
	}).Error; err != nil {
		t.Fatalf("创建礼物流水失败: %v", err)
	}

	items, total, err := svc.ListAdminGrowthUsers(ctx, AdminGrowthUserListFilter{Q: user.Username})
	if err != nil {
		t.Fatalf("查询成长用户列表失败: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].UserNo != user.UserNo {
		t.Fatalf("成长用户列表不符: total=%d items=%+v", total, items)
	}
	if items[0].CurrentXP != 40 || items[0].AchievementCount != 1 || items[0].GiftBalance != 12 {
		t.Fatalf("成长用户摘要不符: %+v", items[0])
	}

	detail, err := svc.GetAdminGrowthUserDetail(ctx, user.UserNo)
	if err != nil {
		t.Fatalf("查询成长用户详情失败: %v", err)
	}
	if detail.User.UserID != user.ID.String() {
		t.Fatalf("详情用户 ID 不符: %+v", detail.User)
	}
	// 只有经验、没有活跃事件的用户仍停留 Lv.0，且不是人工覆盖。
	if detail.User.Level != GuestLevel || detail.User.IsOverridden || detail.User.ActiveDays != 0 {
		t.Fatalf("访客详情等级状态不符: %+v", detail.User)
	}
	if len(detail.Experience) != 2 {
		t.Fatalf("经验流水应有 2 条（人工调整 + 成就奖励），实际 %d", len(detail.Experience))
	}
	if len(detail.Achievements) != 1 || detail.Achievements[0].Code != achievement.Code {
		t.Fatalf("成就详情不符: %+v", detail.Achievements)
	}
	if len(detail.Gifts) != 1 || detail.Gifts[0].Amount != 12 {
		t.Fatalf("礼物详情不符: %+v", detail.Gifts)
	}
	if detail.GiftTotal != 1 || detail.User.GiftBalance != 12 {
		t.Fatalf("礼物统计不符: total=%d balance=%d", detail.GiftTotal, detail.User.GiftBalance)
	}
	if detail.AuditTotal < 2 || len(detail.Audits) < 2 {
		t.Fatalf("成长审计应至少 2 条，实际 total=%d rows=%d", detail.AuditTotal, len(detail.Audits))
	}

	// 管理员账号也必须能在用户详情中检索到；没有成长投影时返回零值详情。
	adminItems, adminTotal, err := svc.ListAdminGrowthUsers(ctx, AdminGrowthUserListFilter{Q: operator.Username})
	if err != nil {
		t.Fatalf("查询管理员成长列表失败: %v", err)
	}
	if adminTotal != 1 || len(adminItems) != 1 || adminItems[0].Role != domain.RoleAdmin {
		t.Fatalf("管理员成长列表不符: total=%d items=%+v", adminTotal, adminItems)
	}
	adminDetail, err := svc.GetAdminGrowthUserDetail(ctx, operator.Username)
	if err != nil {
		t.Fatalf("查询管理员成长详情失败: %v", err)
	}
	if adminDetail.User.UserID != operator.ID.String() || adminDetail.User.Role != domain.RoleAdmin {
		t.Fatalf("管理员详情身份不符: %+v", adminDetail.User)
	}
	if adminDetail.User.CurrentXP != 0 || adminDetail.User.AchievementCount != 0 || adminDetail.User.GiftBalance != 0 {
		t.Fatalf("管理员空成长详情应为零值: %+v", adminDetail.User)
	}
}
