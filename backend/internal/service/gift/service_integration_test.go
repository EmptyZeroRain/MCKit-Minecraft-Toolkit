package gift

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mckit/backend/internal/accountid"
	"mckit/backend/internal/domain"
	db "mckit/backend/internal/repository"
)

const giftSchema = "gift_it"

func withSearchPath(dsn, schema string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + schema
}

func setupGiftDB(t *testing.T) *gorm.DB {
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
	if err := adminDB.Exec("DROP SCHEMA IF EXISTS " + giftSchema + " CASCADE").Error; err != nil {
		t.Fatalf("清理测试 schema 失败: %v", err)
	}
	if err := adminDB.Exec("CREATE SCHEMA " + giftSchema).Error; err != nil {
		t.Fatalf("创建测试 schema 失败: %v", err)
	}
	if sqlDB, err := adminDB.DB(); err == nil {
		_ = sqlDB.Close()
	}
	gdb, err := db.Open(withSearchPath(url, giftSchema))
	if err != nil {
		t.Fatalf("连接测试 schema 失败: %v", err)
	}
	if _, err := db.Migrate(ctx, gdb, "../../../migrations"); err != nil {
		t.Fatalf("执行迁移失败: %v", err)
	}
	return gdb
}

func createGiftUser(t *testing.T, gdb *gorm.DB, username, role string) *domain.User {
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

// TestGiftWalletFlowIntegration 覆盖钱包增减、余额保护、幂等与流水查询。
func TestGiftWalletFlowIntegration(t *testing.T) {
	gdb := setupGiftDB(t)
	ctx := context.Background()
	svc := NewService(gdb)

	user := createGiftUser(t, gdb, "it-gift-user", domain.RoleUser)
	operator := createGiftUser(t, gdb, "it-gift-admin", domain.RoleAdmin)

	entry, err := svc.Adjust(ctx, AdjustInput{
		TargetUserID: user.ID.String(),
		Amount:       100,
		Reason:       "集成测试发放",
		RequestKey:   "grant-1",
		OperatorID:   operator.ID.String(),
	})
	if err != nil {
		t.Fatalf("增加礼物币失败: %v", err)
	}
	if entry.CoinBalanceAfter != 100 || entry.Amount != 100 || entry.CoinType != domain.GiftCoinGeneral {
		t.Fatalf("增加后余额异常: %+v", entry)
	}

	wallet, err := svc.GetWallet(ctx, user.ID.String())
	if err != nil || wallet.GeneralBalance != 100 {
		t.Fatalf("钱包查询异常: balance=%d err=%v", wallet.GeneralBalance, err)
	}
	if wallet.UserNo != user.UserNo {
		t.Fatalf("钱包 user_no = %q，期望 %q", wallet.UserNo, user.UserNo)
	}

	// 相同 request_key 重放不应重复扣减 / 增加。
	if _, err := svc.Adjust(ctx, AdjustInput{
		TargetUserID: user.ID.String(),
		Amount:       100,
		Reason:       "集成测试发放",
		RequestKey:   "grant-1",
		OperatorID:   operator.ID.String(),
	}); err != nil {
		t.Fatalf("幂等重放失败: %v", err)
	}
	wallet, _ = svc.GetWallet(ctx, user.ID.String())
	if wallet.GeneralBalance != 100 {
		t.Fatalf("幂等重放后余额 = %d，期望 100", wallet.GeneralBalance)
	}

	if _, err := svc.Adjust(ctx, AdjustInput{
		TargetUserID: user.ID.String(),
		Amount:       -40,
		Reason:       "集成测试扣减",
		RequestKey:   "deduct-1",
		OperatorID:   operator.ID.String(),
	}); err != nil {
		t.Fatalf("扣减礼物币失败: %v", err)
	}
	wallet, _ = svc.GetWallet(ctx, user.ID.String())
	if wallet.GeneralBalance != 60 {
		t.Fatalf("扣减后余额 = %d，期望 60", wallet.GeneralBalance)
	}

	if _, err := svc.Adjust(ctx, AdjustInput{
		TargetUserID: user.ID.String(),
		Amount:       -100,
		Reason:       "余额不足测试",
		RequestKey:   "deduct-over",
		OperatorID:   operator.ID.String(),
	}); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("余额不足应返回 ErrInsufficientBalance，实际 %v", err)
	}
	wallet, _ = svc.GetWallet(ctx, user.ID.String())
	if wallet.GeneralBalance != 60 {
		t.Fatalf("余额不足失败后余额被改动: %d", wallet.GeneralBalance)
	}

	// 签到币单独记余额，不与通用币互相兜底；后台调整同样支持。
	entry, err = svc.Adjust(ctx, AdjustInput{
		TargetUserID: user.UserNo,
		CoinType:     domain.GiftCoinCheckin,
		Amount:       30,
		Reason:       "签到币测试发放",
		RequestKey:   "checkin-grant-1",
		OperatorID:   operator.ID.String(),
	})
	if err != nil {
		t.Fatalf("增加签到币失败: %v", err)
	}
	if entry.CoinType != domain.GiftCoinCheckin || entry.CoinBalanceAfter != 30 {
		t.Fatalf("签到币流水异常: %+v", entry)
	}
	wallet, _ = svc.GetWallet(ctx, user.ID.String())
	if wallet.GeneralBalance != 60 || wallet.CheckinBalance != 30 || wallet.TotalBalance != 90 {
		t.Fatalf("双币种钱包异常: general=%d checkin=%d total=%d",
			wallet.GeneralBalance, wallet.CheckinBalance, wallet.TotalBalance)
	}

	rows, total, err := svc.ListLedger(ctx, LedgerFilter{UserID: user.UserNo, Page: 1, PageSize: 20})
	if err != nil || total != 3 || len(rows) != 3 {
		t.Fatalf("流水查询异常: total=%d len=%d err=%v", total, len(rows), err)
	}
	checkinRows, checkinTotal, err := svc.ListLedger(ctx, LedgerFilter{
		UserID: user.UserNo, CoinType: domain.GiftCoinCheckin, Page: 1, PageSize: 20,
	})
	if err != nil || checkinTotal != 1 || len(checkinRows) != 1 || checkinRows[0].CoinType != domain.GiftCoinCheckin {
		t.Fatalf("签到币流水筛选异常: total=%d rows=%+v err=%v", checkinTotal, checkinRows, err)
	}
	for _, row := range rows {
		if row.UserID != user.ID {
			t.Fatalf("流水用户不符: %s", row.UserID)
		}
		if row.OperatorAdminID == nil || *row.OperatorAdminID != operator.ID {
			t.Fatalf("流水操作人缺失: %+v", row)
		}
	}

	// 非法输入。
	if _, err := svc.Adjust(ctx, AdjustInput{TargetUserID: uuid.NewString(), Amount: 1, Reason: "x"}); !errors.Is(err, ErrUnknownUser) {
		t.Fatalf("不存在用户应返回 ErrUnknownUser，实际 %v", err)
	}
	if _, err := svc.Adjust(ctx, AdjustInput{TargetUserID: user.ID.String(), Amount: 1, Reason: "  "}); !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("空原因应返回 ErrReasonRequired，实际 %v", err)
	}
	if _, err := svc.Adjust(ctx, AdjustInput{
		TargetUserID: user.ID.String(), CoinType: "cash", Amount: 1, Reason: "非法币种",
	}); !errors.Is(err, ErrBadCoinType) {
		t.Fatalf("非法币种应返回 ErrBadCoinType，实际 %v", err)
	}
}
