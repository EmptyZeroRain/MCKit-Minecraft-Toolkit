package growth

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

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

// TestGrowthAdminControlsIntegration 覆盖单用户等级偏移、成就授予 / 撤销与奖励经验幂等。
func TestGrowthAdminControlsIntegration(t *testing.T) {
	gdb := setupGrowthAdminDB(t)
	ctx := context.Background()
	svc := NewService(gdb, "UTC")

	user := createGrowthUser(t, gdb, "it-growth-user", domain.RoleUser)
	operator := createGrowthUser(t, gdb, "it-growth-admin", domain.RoleAdmin)

	// 初始 Lv.1，+2 后应为 Lv.3，偏移 +2。
	result, err := svc.AdjustLevel(ctx, AdjustLevelInput{
		TargetUserID: user.ID.String(),
		DeltaLevel:   2,
		Reason:       "集成测试升级",
		OperatorID:   operator.ID.String(),
	})
	if err != nil {
		t.Fatalf("等级调整失败: %v", err)
	}
	if result.Level != 3 || result.BaseLevel != 1 || result.LevelOffset != 2 {
		t.Fatalf("等级调整结果异常: %+v", result)
	}
	var g domain.UserGrowth
	if err := gdb.Where("user_id = ?", user.ID).First(&g).Error; err != nil {
		t.Fatalf("读取成长投影失败: %v", err)
	}
	if g.Level != 3 || g.LevelOffset != 2 {
		t.Fatalf("成长投影未持久化: level=%d offset=%d", g.Level, g.LevelOffset)
	}

	// 再 -1，应为 Lv.2，偏移 +1。
	result, err = svc.AdjustLevel(ctx, AdjustLevelInput{
		TargetUserID: user.ID.String(),
		DeltaLevel:   -1,
		Reason:       "集成测试降级",
		OperatorID:   operator.ID.String(),
	})
	if err != nil || result.Level != 2 || result.LevelOffset != 1 {
		t.Fatalf("二次等级调整异常: result=%+v err=%v", result, err)
	}

	// 超出等级范围应拒绝，且不写入。
	if _, err := svc.AdjustLevel(ctx, AdjustLevelInput{
		TargetUserID: user.ID.String(),
		DeltaLevel:   -99,
		Reason:       "越界测试",
		OperatorID:   operator.ID.String(),
	}); !errors.Is(err, ErrLevelRange) {
		t.Fatalf("越界等级应返回 ErrLevelRange，实际 %v", err)
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

// TestGrowthLevelManagementIntegration 覆盖等级新增 / 删除、连续等级约束与删除后的用户等级重算。
func TestGrowthLevelManagementIntegration(t *testing.T) {
	gdb := setupGrowthAdminDB(t)
	ctx := context.Background()
	svc := NewService(gdb, "UTC")

	user := createGrowthUser(t, gdb, "it-level-manage-user", domain.RoleUser)
	operator := createGrowthUser(t, gdb, "it-level-manage-admin", domain.RoleAdmin)

	levels, err := svc.ListLevels(ctx)
	if err != nil || len(levels) != 10 {
		t.Fatalf("初始等级应为 10 个: len=%d err=%v", len(levels), err)
	}

	created, err := svc.CreateLevel(ctx, CreateLevelInput{
		ThresholdXP: 8000,
		Title:       "测试等级",
		Enabled:     true,
	}, operator.ID.String())
	if err != nil {
		t.Fatalf("新增等级失败: %v", err)
	}
	if created.Level != 11 || created.ThresholdXP != 8000 {
		t.Fatalf("新增等级结果异常: %+v", created)
	}

	// 阈值必须高于当前最高等级。
	if _, err := svc.CreateLevel(ctx, CreateLevelInput{
		ThresholdXP: 8000, Title: "重复阈值", Enabled: true,
	}, operator.ID.String()); !errors.Is(err, ErrLevelNewThreshold) {
		t.Fatalf("重复阈值应返回 ErrLevelNewThreshold，实际 %v", err)
	}

	// 只能删除最高等级，Lv.1 永远保留。
	if err := svc.DeleteLevel(ctx, 5, operator.ID.String()); !errors.Is(err, ErrLevelDeleteTopOnly) {
		t.Fatalf("删除中间等级应返回 ErrLevelDeleteTopOnly，实际 %v", err)
	}
	if err := svc.DeleteLevel(ctx, 1, operator.ID.String()); !errors.Is(err, ErrLevelDeleteTopOnly) {
		t.Fatalf("删除 Lv.1 应返回 ErrLevelDeleteTopOnly，实际 %v", err)
	}

	// 用户当前处于 Lv.10；删除新增的 Lv.11 后不受影响。
	if err := gdb.Create(&domain.UserGrowth{
		UserID: user.ID, CurrentXP: 6000, Level: 10,
	}).Error; err != nil {
		t.Fatalf("创建成长投影失败: %v", err)
	}
	if err := svc.DeleteLevel(ctx, 11, operator.ID.String()); err != nil {
		t.Fatalf("删除最高等级失败: %v", err)
	}
	var g domain.UserGrowth
	if err := gdb.Where("user_id = ?", user.ID).First(&g).Error; err != nil {
		t.Fatalf("读取成长投影失败: %v", err)
	}
	if g.Level != 10 {
		t.Fatalf("删除 Lv.11 后用户应保持 Lv.10，实际 %d", g.Level)
	}

	// 删除原最高级 Lv.10 后，用户应按剩余阈值降到 Lv.9。
	if err := svc.DeleteLevel(ctx, 10, operator.ID.String()); err != nil {
		t.Fatalf("删除 Lv.10 失败: %v", err)
	}
	if err := gdb.Where("user_id = ?", user.ID).First(&g).Error; err != nil {
		t.Fatalf("读取成长投影失败: %v", err)
	}
	if g.Level != 9 || g.LevelOffset != 0 {
		t.Fatalf("删除 Lv.10 后用户应重算为 Lv.9，实际 level=%d offset=%d", g.Level, g.LevelOffset)
	}

	// 重新补齐 Lv.10，并通过重算恢复用户等级。
	if _, err := svc.CreateLevel(ctx, CreateLevelInput{
		ThresholdXP: 6000, Title: "MCkit 传奇", Enabled: true,
	}, operator.ID.String()); err != nil {
		t.Fatalf("重新新增 Lv.10 失败: %v", err)
	}
	if fixed, err := svc.RecalculateLevels(ctx); err != nil || fixed != 1 {
		t.Fatalf("重算等级异常: fixed=%d err=%v", fixed, err)
	}
	if err := gdb.Where("user_id = ?", user.ID).First(&g).Error; err != nil {
		t.Fatalf("读取成长投影失败: %v", err)
	}
	if g.Level != 10 {
		t.Fatalf("重算后用户应回到 Lv.10，实际 %d", g.Level)
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
