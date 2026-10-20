package gift

import (
	"context"
	"errors"
	"testing"

	"mckit/backend/internal/domain"
)

// TestGiftCheckinAndRedeemIntegration 覆盖签到、双币种扣减顺序、幂等兑换与目录管理。
func TestGiftCheckinAndRedeemIntegration(t *testing.T) {
	gdb := setupGiftDB(t)
	ctx := context.Background()
	svc := NewService(gdb).WithTimezone("Asia/Shanghai")

	user := createGiftUser(t, gdb, "it-gift-shop-user", domain.RoleUser)

	// 默认签到奖励为 1；后台可调整。
	settings, err := svc.GetSettings(ctx)
	if err != nil || settings.CheckinReward != 1 {
		t.Fatalf("默认签到奖励异常: %+v err=%v", settings, err)
	}
	if _, err := svc.UpdateSettings(ctx, 5); err != nil {
		t.Fatalf("更新签到奖励失败: %v", err)
	}
	if _, err := svc.UpdateSettings(ctx, -1); !errors.Is(err, ErrCheckinReward) {
		t.Fatalf("非法签到奖励应返回 ErrCheckinReward，实际 %v", err)
	}

	// 签到一次发 5 签到币；重复签到被拒绝。
	result, err := svc.Checkin(ctx, user.ID.String())
	if err != nil {
		t.Fatalf("签到失败: %v", err)
	}
	if result.Reward != 5 || result.Wallet.CheckinBalance != 5 || result.Wallet.GeneralBalance != 0 {
		t.Fatalf("签到结果异常: %+v", result)
	}
	if _, err := svc.Checkin(ctx, user.ID.String()); !errors.Is(err, ErrAlreadyCheckedIn) {
		t.Fatalf("重复签到应返回 ErrAlreadyCheckedIn，实际 %v", err)
	}

	catalog, err := svc.Catalog(ctx, user.ID.String())
	if err != nil {
		t.Fatalf("目录加载失败: %v", err)
	}
	byCode := map[string]CatalogItem{}
	for _, item := range catalog {
		byCode[item.Code] = item
	}
	dirt, ok := byCode["dirt_block"]
	if !ok || !dirt.AllowCheckin || dirt.Price != 1 {
		t.Fatalf("初始礼物泥土块异常: %+v", dirt)
	}
	diamond, ok := byCode["diamond"]
	if !ok || diamond.AllowCheckin || diamond.CanAfford {
		t.Fatalf("初始礼物钻石异常: %+v", diamond)
	}

	// 允许签到币：先扣签到币（泥土块 1 币）。
	redeemed, err := svc.Redeem(ctx, RedeemInput{
		UserID: user.ID.String(), GiftID: dirt.ID, RequestKey: "redeem-dirt-1",
	})
	if err != nil {
		t.Fatalf("兑换泥土块失败: %v", err)
	}
	if redeemed.Redemption.CheckinSpent != 1 || redeemed.Redemption.GeneralSpent != 0 {
		t.Fatalf("泥土块扣减异常: %+v", redeemed.Redemption)
	}
	if redeemed.Wallet.CheckinBalance != 4 || redeemed.Wallet.GeneralBalance != 0 {
		t.Fatalf("泥土块兑换后余额异常: %+v", redeemed.Wallet)
	}

	// 幂等重放：同一 request_key 不再扣币。
	replay, err := svc.Redeem(ctx, RedeemInput{
		UserID: user.ID.String(), GiftID: dirt.ID, RequestKey: "redeem-dirt-1",
	})
	if err != nil {
		t.Fatalf("幂等重放失败: %v", err)
	}
	if replay.Redemption.ID != redeemed.Redemption.ID || replay.Wallet.CheckinBalance != 4 {
		t.Fatalf("幂等重放结果异常: %+v", replay)
	}

	// 余额不足：钻石只能用通用币，当前通用币为 0。
	if _, err := svc.Redeem(ctx, RedeemInput{
		UserID: user.ID.String(), GiftID: diamond.ID, RequestKey: "redeem-diamond-1",
	}); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("通用币不足应返回 ErrInsufficientBalance，实际 %v", err)
	}

	// 后台发放通用币后，钻石只能扣通用币，签到币保持不变。
	if _, err := svc.Adjust(ctx, AdjustInput{
		TargetUserID: user.ID.String(),
		CoinType:     domain.GiftCoinGeneral,
		Amount:       60,
		Reason:       "活动发放",
		RequestKey:   "grant-general-1",
	}); err != nil {
		t.Fatalf("发放通用币失败: %v", err)
	}
	redeemed, err = svc.Redeem(ctx, RedeemInput{
		UserID: user.ID.String(), GiftID: diamond.ID, RequestKey: "redeem-diamond-2",
	})
	if err != nil {
		t.Fatalf("兑换钻石失败: %v", err)
	}
	if redeemed.Redemption.CheckinSpent != 0 || redeemed.Redemption.GeneralSpent != 50 {
		t.Fatalf("钻石扣减异常: %+v", redeemed.Redemption)
	}
	if redeemed.Wallet.CheckinBalance != 4 || redeemed.Wallet.GeneralBalance != 10 {
		t.Fatalf("钻石兑换后余额异常: %+v", redeemed.Wallet)
	}

	redemptions, total, err := svc.ListSelfRedemptions(ctx, user.ID.String(), 1, 20)
	if err != nil || total != 2 || len(redemptions) != 2 {
		t.Fatalf("兑换记录异常: total=%d len=%d err=%v", total, len(redemptions), err)
	}

	// 目录管理：新建 -> 编辑 -> 删除；已兑换礼物只能停用。
	created, err := svc.CreateDefinition(ctx, DefinitionInput{
		Code: "test_gift", Name: "测试礼物", Description: "集成测试", Icon: "🧪",
		Price: 2, AllowCheckin: true, SortOrder: 5, Enabled: true,
	})
	if err != nil {
		t.Fatalf("新建礼物失败: %v", err)
	}
	if created.Code != "test_gift" || created.Price != 2 || !created.AllowCheckin {
		t.Fatalf("新建礼物结果异常: %+v", created)
	}
	updated, err := svc.UpdateDefinition(ctx, created.ID, DefinitionInput{
		Name: "测试礼物（改）", Description: "已编辑", Icon: "🧪",
		Price: 3, AllowCheckin: false, SortOrder: 6, Enabled: false,
	})
	if err != nil {
		t.Fatalf("编辑礼物失败: %v", err)
	}
	if updated.Price != 3 || updated.Enabled || updated.AllowCheckin {
		t.Fatalf("编辑礼物结果异常: %+v", updated)
	}
	if err := svc.DeleteDefinition(ctx, created.ID); err != nil {
		t.Fatalf("删除未兑换礼物失败: %v", err)
	}
	if err := svc.DeleteDefinition(ctx, dirt.ID); !errors.Is(err, ErrGiftInUse) {
		t.Fatalf("已兑换礼物删除应返回 ErrGiftInUse，实际 %v", err)
	}

	// 重复编码被拒绝。
	if _, err := svc.CreateDefinition(ctx, DefinitionInput{
		Code: "dirt_block", Name: "重复", Price: 1, Enabled: true,
	}); !errors.Is(err, ErrGiftCodeTaken) {
		t.Fatalf("重复编码应返回 ErrGiftCodeTaken，实际 %v", err)
	}

	// 停用后的礼物不可兑换。
	if _, err := svc.UpdateDefinition(ctx, dirt.ID, DefinitionInput{
		Name: dirt.Name, Description: dirt.Description, Icon: dirt.Icon,
		Price: dirt.Price, AllowCheckin: dirt.AllowCheckin, SortOrder: dirt.SortOrder, Enabled: false,
	}); err != nil {
		t.Fatalf("停用礼物失败: %v", err)
	}
	if _, err := svc.Redeem(ctx, RedeemInput{
		UserID: user.ID.String(), GiftID: dirt.ID, RequestKey: "redeem-disabled",
	}); !errors.Is(err, ErrGiftDisabled) {
		t.Fatalf("停用礼物兑换应返回 ErrGiftDisabled，实际 %v", err)
	}
}

// TestGiftRankingIntegration 覆盖签到 / 礼物 / 礼物币三类排行榜的统计与分页。
func TestGiftRankingIntegration(t *testing.T) {
	gdb := setupGiftDB(t)
	ctx := context.Background()
	svc := NewService(gdb).WithTimezone("Asia/Shanghai")

	userA := createGiftUser(t, gdb, "it-rank-a", domain.RoleUser)
	userB := createGiftUser(t, gdb, "it-rank-b", domain.RoleUser)
	_ = createGiftUser(t, gdb, "it-rank-admin", domain.RoleAdmin)

	today := svc.businessDate()
	yesterday := today.AddDate(0, 0, -1)
	checkins := []domain.GiftCheckin{
		{UserID: userA.ID, CheckinDate: today, Reward: 5},
		{UserID: userA.ID, CheckinDate: yesterday, Reward: 3},
		{UserID: userB.ID, CheckinDate: today, Reward: 5},
	}
	if err := gdb.Create(&checkins).Error; err != nil {
		t.Fatalf("写入签到记录失败: %v", err)
	}

	items, total, err := svc.CheckinRanking(ctx, RankingFilter{Days: 0, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("签到排行榜查询失败: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("签到排行榜数量异常: total=%d len=%d", total, len(items))
	}
	if items[0].UserID != userA.ID.String() || items[0].CheckinDays != 2 || items[0].CheckinReward != 8 {
		t.Fatalf("签到排行榜首位异常: %+v", items[0])
	}
	if items[1].UserID != userB.ID.String() || items[1].CheckinDays != 1 {
		t.Fatalf("签到排行榜次位异常: %+v", items[1])
	}

	// 近 1 天：A / B 各 1 次，管理员不计入。
	items, total, err = svc.CheckinRanking(ctx, RankingFilter{Days: 1, Page: 1, PageSize: 20})
	if err != nil || total != 2 || len(items) != 2 {
		t.Fatalf("近 1 天签到排行榜异常: total=%d len=%d err=%v", total, len(items), err)
	}

	var dirt domain.GiftDefinition
	if err := gdb.Where("code = ?", "dirt_block").First(&dirt).Error; err != nil {
		t.Fatalf("查询初始礼物失败: %v", err)
	}
	var coal domain.GiftDefinition
	if err := gdb.Where("code = ?", "coal").First(&coal).Error; err != nil {
		t.Fatalf("查询初始礼物失败: %v", err)
	}
	redemptions := []domain.GiftRedemption{
		{UserID: userA.ID, GiftID: dirt.ID, GiftCodeSnapshot: dirt.Code, GiftNameSnapshot: dirt.Name, GiftIconSnapshot: dirt.Icon, Price: dirt.Price, CheckinSpent: dirt.Price, IdempotencyKey: "rank-a-dirt"},
		{UserID: userB.ID, GiftID: dirt.ID, GiftCodeSnapshot: dirt.Code, GiftNameSnapshot: dirt.Name, GiftIconSnapshot: dirt.Icon, Price: dirt.Price, CheckinSpent: dirt.Price, IdempotencyKey: "rank-b-dirt"},
		{UserID: userA.ID, GiftID: coal.ID, GiftCodeSnapshot: coal.Code, GiftNameSnapshot: coal.Name, GiftIconSnapshot: coal.Icon, Price: coal.Price, GeneralSpent: coal.Price, IdempotencyKey: "rank-a-coal"},
	}
	if err := gdb.Create(&redemptions).Error; err != nil {
		t.Fatalf("写入兑换记录失败: %v", err)
	}

	giftItems, giftTotal, err := svc.GiftRanking(ctx, RankingFilter{Days: 0, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("礼物排行榜查询失败: %v", err)
	}
	if giftTotal != 2 || len(giftItems) != 2 {
		t.Fatalf("礼物排行榜数量异常: total=%d len=%d", giftTotal, len(giftItems))
	}
	if giftItems[0].Code != "dirt_block" || giftItems[0].RedemptionCount != 2 || giftItems[0].TotalCoins != 2 {
		t.Fatalf("礼物排行榜首位异常: %+v", giftItems[0])
	}

	if _, err := svc.Adjust(ctx, AdjustInput{
		TargetUserID: userA.ID.String(), Amount: 100, Reason: "排行榜测试", RequestKey: "rank-grant-a",
	}); err != nil {
		t.Fatalf("发放通用币失败: %v", err)
	}
	if _, err := svc.Adjust(ctx, AdjustInput{
		TargetUserID: userB.ID.String(), CoinType: domain.GiftCoinCheckin, Amount: 20,
		Reason: "排行榜测试", RequestKey: "rank-grant-b",
	}); err != nil {
		t.Fatalf("发放签到币失败: %v", err)
	}

	coinItems, coinTotal, err := svc.CoinRanking(ctx, RankingFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("礼物币排行榜查询失败: %v", err)
	}
	if coinTotal != 2 || len(coinItems) != 2 {
		t.Fatalf("礼物币排行榜数量异常: total=%d len=%d", coinTotal, len(coinItems))
	}
	if coinItems[0].UserID != userA.ID.String() || coinItems[0].TotalBalance != 100 {
		t.Fatalf("礼物币总榜首位异常: %+v", coinItems[0])
	}
	coinItems, checkinTotal, err := svc.CoinRanking(ctx, RankingFilter{CoinType: domain.GiftCoinCheckin, Page: 1, PageSize: 20})
	if err != nil || checkinTotal != 1 || len(coinItems) != 1 || coinItems[0].UserID != userB.ID.String() || coinItems[0].CheckinBalance != 20 {
		t.Fatalf("签到币排行榜异常: total=%d items=%+v err=%v", checkinTotal, coinItems, err)
	}

	// 分页只影响当页条目，total 仍为全量去重数量。
	page, pageTotal, err := svc.CheckinRanking(ctx, RankingFilter{Days: 0, Page: 2, PageSize: 1})
	if err != nil || pageTotal != 2 || len(page) != 1 || page[0].Rank != 2 {
		t.Fatalf("签到排行榜分页异常: total=%d items=%+v err=%v", pageTotal, page, err)
	}
}
