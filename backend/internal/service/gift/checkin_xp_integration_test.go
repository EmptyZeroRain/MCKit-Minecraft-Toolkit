package gift

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"mckit/backend/internal/domain"
	growthsvc "mckit/backend/internal/service/growth"
)

// TestCheckinXPAndMakeupCardsIntegration 覆盖签到经验公式、里程碑、断签惩罚与补签卡。
func TestCheckinXPAndMakeupCardsIntegration(t *testing.T) {
	gdb := setupGiftDB(t)
	ctx := context.Background()
	growth := growthsvc.NewService(gdb, "Asia/Shanghai")
	svc := NewService(gdb).WithTimezone("Asia/Shanghai").WithGrowthSink(growth)

	user := createGiftUser(t, gdb, "it-checkin-xp-user", domain.RoleUser)
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("加载时区失败: %v", err)
	}
	day := func(y int, m time.Month, d int) time.Time {
		return time.Date(y, m, d, 12, 0, 0, 0, loc)
	}
	svc.now = func() time.Time { return day(2026, 1, 10) }

	reward := int64(100)
	base := int64(10)
	step := int64(2)
	cap := int64(100)
	milestoneCap := int64(500)
	penalty := int64(5)
	price := int64(10)
	milestones := []MilestoneInput{{StreakDays: 3, BonusXP: 10, Enabled: true}}
	settings, err := svc.UpdateCheckinSettings(ctx, CheckinSettingsInput{
		CheckinReward: &reward, BaseXP: &base, StreakStepXP: &step,
		StreakBonusCap: &cap, MilestoneBonusCap: &milestoneCap,
		BreakPenaltyXP: &penalty, MakeupCardPrice: &price, Milestones: &milestones,
	})
	if err != nil {
		t.Fatalf("更新签到配置失败: %v", err)
	}
	if len(settings.Milestones) != 1 || settings.Milestones[0].StreakDays != 3 {
		t.Fatalf("里程碑配置异常: %+v", settings.Milestones)
	}

	// 首次签到前预览：连续 1 天、基础 10 经验，无断签惩罚。
	status, err := svc.CheckinStatus(ctx, user.ID.String())
	if err != nil {
		t.Fatalf("查询签到状态失败: %v", err)
	}
	if status.CheckedIn || status.Streak != 0 || status.NextStreak != 1 ||
		status.TodayXP != 10 || status.PenaltyXP != 0 || status.LatestMissedDate != "" {
		t.Fatalf("首次签到前状态异常: %+v", status)
	}

	// 连续签到 3 天：基础 + 连续递增 + 3 天里程碑。
	sign := func(at time.Time) *CheckinResult {
		t.Helper()
		svc.now = func() time.Time { return at }
		out, err := svc.Checkin(ctx, user.ID.String())
		if err != nil {
			t.Fatalf("%s 签到失败: %v", at.Format("2006-01-02"), err)
		}
		return out
	}
	d1 := sign(day(2026, 1, 10))
	if d1.Streak != 1 || d1.XP != 10 || d1.BaseXP != 10 || d1.StreakXP != 0 || d1.MilestoneXP != 0 || d1.PenaltyXP != 0 {
		t.Fatalf("第 1 天经验异常: %+v", d1)
	}
	d2 := sign(day(2026, 1, 11))
	if d2.Streak != 2 || d2.XP != 12 || d2.StreakXP != 2 {
		t.Fatalf("第 2 天经验异常: %+v", d2)
	}
	d3 := sign(day(2026, 1, 12))
	if d3.Streak != 3 || d3.XP != 24 || d3.StreakXP != 4 || d3.MilestoneXP != 10 {
		t.Fatalf("第 3 天经验异常: %+v", d3)
	}

	// 漏签后、签到前预览：连续天数重置为 1，提示最近漏签日与断签惩罚。
	svc.now = func() time.Time { return day(2026, 1, 14) }
	status, err = svc.CheckinStatus(ctx, user.ID.String())
	if err != nil {
		t.Fatalf("查询漏签状态失败: %v", err)
	}
	if status.Streak != 3 || status.NextStreak != 1 || status.PenaltyXP != 5 ||
		status.LatestMissedDate != "2026-01-13" {
		t.Fatalf("漏签状态异常: %+v", status)
	}

	// 漏签 1 天后签到：连续天数重置为 1，并扣一次断签惩罚。
	d5 := sign(day(2026, 1, 14))
	if d5.Streak != 1 || d5.XP != 5 || d5.PenaltyXP != 5 || d5.Wallet.CheckinBalance != 400 {
		t.Fatalf("断签后签到经验异常: %+v", d5)
	}
	var xp int64
	if err := gdb.Model(&domain.ExperienceLedger{}).
		Where("user_id = ?", user.ID).
		Select("COALESCE(SUM(delta_xp), 0)").Scan(&xp).Error; err != nil {
		t.Fatalf("查询经验流水失败: %v", err)
	}
	if xp != 51 {
		t.Fatalf("累计经验应为 10+12+24+5=51，实际 %d", xp)
	}

	// 补签卡购买幂等：同一 request_key 不重复扣签到币。
	buy, err := svc.PurchaseMakeupCard(ctx, user.ID.String(), "buy-1")
	if err != nil {
		t.Fatalf("购买补签卡失败: %v", err)
	}
	if buy.Cards != 1 || buy.Price != price || buy.Wallet.CheckinBalance != 390 {
		t.Fatalf("购买补签卡结果异常: %+v", buy)
	}
	replayBuy, err := svc.PurchaseMakeupCard(ctx, user.ID.String(), "buy-1")
	if err != nil {
		t.Fatalf("幂等购买失败: %v", err)
	}
	if replayBuy.Cards != 1 || replayBuy.Wallet.CheckinBalance != 390 {
		t.Fatalf("幂等购买重复扣币: %+v", replayBuy)
	}
	if _, err := svc.PurchaseMakeupCard(ctx, user.ID.String(), "buy-2"); err != nil {
		t.Fatalf("第二次购买补签卡失败: %v", err)
	}

	// 补签最近漏签日（1 月 13 日）：只补经验，不写签到天数、不改变连续天数。
	svc.now = func() time.Time { return day(2026, 1, 14) }
	use, err := svc.UseMakeupCard(ctx, user.ID.String(), "use-1")
	if err != nil {
		t.Fatalf("使用补签卡失败: %v", err)
	}
	if use.Date != "2026-01-13" || use.BaseXP != 10 || use.StreakXP != 6 ||
		use.MilestoneXP != 0 || use.XP != 16 || use.Cards != 1 {
		t.Fatalf("补签经验异常: %+v", use)
	}
	var checkinCount int64
	if err := gdb.Model(&domain.GiftCheckin{}).Where("user_id = ?", user.ID).Count(&checkinCount).Error; err != nil {
		t.Fatalf("查询签到天数失败: %v", err)
	}
	if checkinCount != 4 {
		t.Fatalf("补签不应增加签到天数，应为 4，实际 %d", checkinCount)
	}
	var streak domain.GiftCheckinStreak
	if err := gdb.Where("user_id = ?", user.ID).First(&streak).Error; err != nil {
		t.Fatalf("查询连续签到失败: %v", err)
	}
	if streak.CurrentStreak != 1 {
		t.Fatalf("补签不应恢复连续天数，实际 %d", streak.CurrentStreak)
	}

	// 同一 request_key 重放不重复消耗补签卡。
	replayUse, err := svc.UseMakeupCard(ctx, user.ID.String(), "use-1")
	if err != nil {
		t.Fatalf("幂等补签失败: %v", err)
	}
	if replayUse.Date != use.Date || replayUse.XP != use.XP || replayUse.Cards != 1 {
		t.Fatalf("幂等补签结果异常: %+v", replayUse)
	}

	// 已无漏签日：不消耗补签卡。
	if _, err := svc.UseMakeupCard(ctx, user.ID.String(), "use-2"); !errors.Is(err, ErrNoMissedCheckin) {
		t.Fatalf("无漏签日应返回 ErrNoMissedCheckin，实际 %v", err)
	}
	var cards domain.GiftMakeupCard
	if err := gdb.Where("user_id = ?", user.ID).First(&cards).Error; err != nil {
		t.Fatalf("查询补签卡失败: %v", err)
	}
	if cards.Quantity != 1 {
		t.Fatalf("失败补签不应扣卡，剩余应为 1，实际 %d", cards.Quantity)
	}
	status, err = svc.CheckinStatus(ctx, user.ID.String())
	if err != nil {
		t.Fatalf("补签后查询状态失败: %v", err)
	}
	if status.LatestMissedDate != "" || status.MakeupCards != 1 || status.Streak != 1 {
		t.Fatalf("补签后状态异常: %+v", status)
	}

	// 只能用签到币购买：只有通用币时购买失败。
	other := createGiftUser(t, gdb, "it-checkin-xp-general-only", domain.RoleUser)
	if _, err := svc.Adjust(ctx, AdjustInput{
		TargetUserID: other.ID.String(), CoinType: domain.GiftCoinGeneral,
		Amount: 100, Reason: "通用币测试", RequestKey: "general-only",
	}); err != nil {
		t.Fatalf("发放通用币失败: %v", err)
	}
	if _, err := svc.PurchaseMakeupCard(ctx, other.ID.String(), "buy-general"); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("通用币不能购买补签卡，实际 %v", err)
	}

	// Lv.0 期间签到经验先攒着：累计满 7 个活跃日后，原经验自动参与升级。
	for _, activeDate := range []time.Time{
		day(2026, 1, 7), day(2026, 1, 8), day(2026, 1, 9),
	} {
		if err := growth.HandleEvent(ctx, domain.GrowthEvent{
			EventID:      uuid.New(),
			EventType:    domain.EventUserRegistered,
			ActorUserID:  user.ID,
			SourceModule: "it",
			OccurredAt:   activeDate,
		}); err != nil {
			t.Fatalf("%s 活跃事件失败: %v", activeDate.Format("2006-01-02"), err)
		}
	}
	var guestProgress domain.UserGrowth
	if err := gdb.Where("user_id = ?", user.ID).First(&guestProgress).Error; err != nil {
		t.Fatalf("查询访客经验失败: %v", err)
	}
	if guestProgress.Level < 1 {
		t.Fatalf("累计满 7 个活跃日后应进入 Lv.1，实际 Lv.%d", guestProgress.Level)
	}
	if guestProgress.CurrentXP != 67 {
		t.Fatalf("Lv.0 期间积攒的签到经验应保留 67，实际 %d", guestProgress.CurrentXP)
	}
}
