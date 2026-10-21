package gift

import (
	"context"
	"errors"
	"testing"

	"mckit/backend/internal/domain"
)

// TestMakeupCardCatalogIntegration 验证补签卡已纳入礼品目录：
// 价格由目录维护、只能签到币购买、购买进个人库存、不出现在普通礼物历史与排行榜。
func TestMakeupCardCatalogIntegration(t *testing.T) {
	gdb := setupGiftDB(t)
	ctx := context.Background()
	svc := NewService(gdb).WithTimezone("Asia/Shanghai")

	user := createGiftUser(t, gdb, "it-makeup-catalog-user", domain.RoleUser)

	// 迁移应种子化唯一的补签卡目录项。
	defs, err := svc.ListDefinitions(ctx)
	if err != nil {
		t.Fatalf("后台目录加载失败: %v", err)
	}
	var def *DefinitionView
	for i := range defs {
		if defs[i].Kind == domain.GiftKindMakeupCard {
			def = &defs[i]
			break
		}
	}
	if def == nil {
		t.Fatal("礼品目录中缺少补签卡目录项")
	}
	if def.Code != "makeup_card" || def.Price != 10 || !def.AllowCheckin || def.Stock != 0 {
		t.Fatalf("补签卡目录项默认值异常: %+v", def)
	}

	// 用户目录能看到补签卡，但当前没有签到币，不可购买。
	catalog, err := svc.Catalog(ctx, user.ID.String())
	if err != nil {
		t.Fatalf("用户目录加载失败: %v", err)
	}
	var item *CatalogItem
	for i := range catalog {
		if catalog[i].Kind == domain.GiftKindMakeupCard {
			item = &catalog[i]
			break
		}
	}
	if item == nil || item.CanAfford {
		t.Fatalf("补签卡用户目录状态异常: %+v", item)
	}

	// 后台改价；库存 / 签到币开关应被强制为签到道具的规范值。
	updated, err := svc.UpdateDefinition(ctx, def.ID, DefinitionInput{
		Name: def.Name, Description: def.Description, Icon: def.Icon,
		Price: 25, Stock: 99, AllowCheckin: false,
		SortOrder: def.SortOrder, Enabled: true,
	})
	if err != nil {
		t.Fatalf("更新补签卡目录失败: %v", err)
	}
	if updated.Price != 25 || updated.Stock != 0 || !updated.AllowCheckin {
		t.Fatalf("补签卡目录更新结果异常: %+v", updated)
	}

	status, err := svc.CheckinStatus(ctx, user.ID.String())
	if err != nil {
		t.Fatalf("查询签到状态失败: %v", err)
	}
	if status.MakeupCardPrice != 25 {
		t.Fatalf("签到状态中的补签卡价格 = %d，期望 25", status.MakeupCardPrice)
	}

	// 通用币充足也不能买补签卡。
	if _, err := svc.Adjust(ctx, AdjustInput{
		TargetUserID: user.ID.String(),
		CoinType:     domain.GiftCoinGeneral,
		Amount:       100,
		Reason:       "通用币补偿",
		RequestKey:   "grant-general",
	}); err != nil {
		t.Fatalf("发放通用币失败: %v", err)
	}
	if _, err := svc.PurchaseMakeupCard(ctx, user.ID.String(), "buy-general"); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("只有通用币时应返回 ErrInsufficientBalance，实际 %v", err)
	}

	// 发放签到币后购买成功：扣 25 签到币、+1 张。
	if _, err := svc.Adjust(ctx, AdjustInput{
		TargetUserID: user.ID.String(),
		CoinType:     domain.GiftCoinCheckin,
		Amount:       30,
		Reason:       "签到币补偿",
		RequestKey:   "grant-checkin",
	}); err != nil {
		t.Fatalf("发放签到币失败: %v", err)
	}
	buy, err := svc.PurchaseMakeupCard(ctx, user.ID.String(), "buy-makeup")
	if err != nil {
		t.Fatalf("购买补签卡失败: %v", err)
	}
	if buy.Price != 25 || buy.Cards != 1 || buy.Wallet.CheckinBalance != 5 || buy.Wallet.GeneralBalance != 100 {
		t.Fatalf("购买补签卡结果异常: %+v", buy)
	}

	var redemption domain.GiftRedemption
	if err := gdb.Where("user_id = ? AND idempotency_key = ?", user.ID, "buy-makeup").
		First(&redemption).Error; err != nil {
		t.Fatalf("补签卡购买应写入兑换记录: %v", err)
	}
	if redemption.CheckinSpent != 25 || redemption.GeneralSpent != 0 {
		t.Fatalf("补签卡兑换记录扣减异常: %+v", redemption)
	}

	// 「我的礼物」与礼物排行榜只统计普通礼物。
	list, total, err := svc.ListSelfRedemptions(ctx, user.ID.String(), 1, 20)
	if err != nil {
		t.Fatalf("我的礼物加载失败: %v", err)
	}
	if total != 0 || len(list) != 0 {
		t.Fatalf("补签卡购买不应出现在我的礼物: total=%d items=%+v", total, list)
	}
	if _, total, err := svc.GiftRanking(ctx, RankingFilter{}); err != nil || total != 0 {
		t.Fatalf("补签卡不应进入礼物排行榜: total=%d err=%v", total, err)
	}

	// 签到道具禁止删除，只能停用。
	if err := svc.DeleteDefinition(ctx, def.ID); !errors.Is(err, ErrGiftSystemItem) {
		t.Fatalf("删除补签卡应返回 ErrGiftSystemItem，实际 %v", err)
	}
}
