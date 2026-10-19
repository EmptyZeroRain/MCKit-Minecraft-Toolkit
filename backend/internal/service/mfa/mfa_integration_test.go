package mfa_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	mfacrypto "mckit/backend/internal/mfa"
	db "mckit/backend/internal/repository"
	mfasvc "mckit/backend/internal/service/mfa"
)

// mfaSchema 本包集成测试专用 schema（与其他包隔离，避免互相 DROP）。
const mfaSchema = "mfa_it"

func withSearchPath(dsn, schema string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + schema
}

func setupMFA(t *testing.T) *gorm.DB {
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
	if err := adminDB.Exec("DROP SCHEMA IF EXISTS " + mfaSchema + " CASCADE").Error; err != nil {
		t.Fatalf("清理测试 schema 失败: %v", err)
	}
	if err := adminDB.Exec("CREATE SCHEMA " + mfaSchema).Error; err != nil {
		t.Fatalf("创建测试 schema 失败: %v", err)
	}
	if sqlDB, err := adminDB.DB(); err == nil {
		_ = sqlDB.Close()
	}

	gdb, err := db.Open(withSearchPath(url, mfaSchema))
	if err != nil {
		t.Fatalf("连接测试 schema 失败: %v", err)
	}
	if _, err := db.Migrate(ctx, gdb, "../../../migrations"); err != nil {
		t.Fatalf("执行迁移失败: %v", err)
	}
	return gdb
}

// seedAccount 建一个启用状态的后台账号，返回其 ID 与用户名。
func seedAccount(t *testing.T, gdb *gorm.DB, username string) uuid.UUID {
	t.Helper()
	acc := domain.Account{
		Username:     username,
		PasswordHash: "$argon2id$dummy",
		Role:         domain.RoleEditor,
		IsActive:     true,
	}
	if err := gdb.Create(&acc).Error; err != nil {
		t.Fatalf("创建账号失败: %v", err)
	}
	return acc.ID
}

// testKey 固定 32 字节密钥。
var testKey = []byte("0123456789abcdef0123456789abcdef")

func newSvc(gdb *gorm.DB) *mfasvc.Service {
	return mfasvc.NewService(gdb, testKey, "MC 工具站测试")
}

func TestSetupEnableVerifyFlow(t *testing.T) {
	gdb := setupMFA(t)
	ctx := context.Background()
	svc := newSvc(gdb)
	accountID := seedAccount(t, gdb, "mfa_admin")

	status, err := svc.Status(ctx, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled || status.Pending {
		t.Fatalf("初始状态应为未绑定: %+v", status)
	}

	setup, err := svc.Setup(ctx, accountID, "mfa_admin")
	if err != nil {
		t.Fatalf("Setup 失败: %v", err)
	}
	if setup.Secret == "" || !strings.HasPrefix(setup.URI, "otpauth://totp/") {
		t.Fatalf("Setup 返回不完整: %+v", setup)
	}

	// 待启用状态：登录不应要求第二因子。
	enabled, err := svc.IsEnabled(ctx, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("待启用状态不应视为已启用")
	}
	status, err = svc.Status(ctx, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Pending || status.Enabled {
		t.Fatalf("状态应为待启用: %+v", status)
	}

	// 错误验证码不能激活。
	if _, err := svc.Enable(ctx, accountID, "000000"); !errors.Is(err, mfasvc.ErrInvalidCode) {
		t.Fatalf("错误验证码应返回 ErrInvalidCode，实际 %v", err)
	}

	code, err := mfacrypto.CodeAt(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	codes, err := svc.Enable(ctx, accountID, code)
	if err != nil {
		t.Fatalf("Enable 失败: %v", err)
	}
	if len(codes) != mfacrypto.RecoveryCodeCount {
		t.Fatalf("恢复码数量 = %d, want %d", len(codes), mfacrypto.RecoveryCodeCount)
	}

	enabled, err = svc.IsEnabled(ctx, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Fatal("激活后应为已启用")
	}

	status, err = svc.Status(ctx, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || status.RecoveryCodesLeft != mfacrypto.RecoveryCodeCount {
		t.Fatalf("状态异常: %+v", status)
	}
}

func TestVerifyRejectsReplayOfSameStep(t *testing.T) {
	gdb := setupMFA(t)
	ctx := context.Background()
	svc := newSvc(gdb)
	accountID := seedAccount(t, gdb, "mfa_replay")

	setup, err := svc.Setup(ctx, accountID, "mfa_replay")
	if err != nil {
		t.Fatal(err)
	}
	code, err := mfacrypto.CodeAt(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Enable(ctx, accountID, code); err != nil {
		t.Fatal(err)
	}

	// Enable 已消费该时间步，再次使用同一验证码必须被拒绝。
	if err := svc.Verify(ctx, accountID, code); !errors.Is(err, mfasvc.ErrInvalidCode) {
		t.Fatalf("同一时间步重放应被拒绝，实际 %v", err)
	}
}

func TestRecoveryCodeIsSingleUse(t *testing.T) {
	gdb := setupMFA(t)
	ctx := context.Background()
	svc := newSvc(gdb)
	accountID := seedAccount(t, gdb, "mfa_recovery")

	setup, err := svc.Setup(ctx, accountID, "mfa_recovery")
	if err != nil {
		t.Fatal(err)
	}
	code, err := mfacrypto.CodeAt(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	codes, err := svc.Enable(ctx, accountID, code)
	if err != nil {
		t.Fatal(err)
	}

	// 用户输入可能带连字符 / 小写，归一化后仍应命中。
	if err := svc.Verify(ctx, accountID, strings.ToLower(codes[0])); err != nil {
		t.Fatalf("恢复码首次使用应成功: %v", err)
	}
	if err := svc.Verify(ctx, accountID, codes[0]); !errors.Is(err, mfasvc.ErrInvalidCode) {
		t.Fatalf("恢复码不可重复使用，实际 %v", err)
	}

	status, err := svc.Status(ctx, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if status.RecoveryCodesLeft != mfacrypto.RecoveryCodeCount-1 {
		t.Fatalf("剩余恢复码 = %d, want %d", status.RecoveryCodesLeft, mfacrypto.RecoveryCodeCount-1)
	}
}

func TestTicketRedeemAndAttemptLimit(t *testing.T) {
	gdb := setupMFA(t)
	ctx := context.Background()
	svc := newSvc(gdb)
	accountID := seedAccount(t, gdb, "mfa_ticket")

	setup, err := svc.Setup(ctx, accountID, "mfa_ticket")
	if err != nil {
		t.Fatal(err)
	}
	code, err := mfacrypto.CodeAt(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Enable(ctx, accountID, code); err != nil {
		t.Fatal(err)
	}

	ticket, err := svc.CreateTicket(ctx, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if ticket == "" {
		t.Fatal("票据为空")
	}

	// 连续错误：累计 attempts，达到上限后票据作废。
	for i := 0; i < mfasvc.MaxTicketAttempts; i++ {
		if _, err := svc.RedeemTicket(ctx, ticket, "000000"); !errors.Is(err, mfasvc.ErrInvalidCode) {
			t.Fatalf("第 %d 次错误验证码应返回 ErrInvalidCode，实际 %v", i+1, err)
		}
	}
	if _, err := svc.RedeemTicket(ctx, ticket, code); !errors.Is(err, mfasvc.ErrTicketInvalid) {
		t.Fatalf("达到尝试上限后票据应作废，实际 %v", err)
	}

	// 新票据 + 正确验证码：成功且只能用一次。
	ticket2, err := svc.CreateTicket(ctx, accountID)
	if err != nil {
		t.Fatal(err)
	}
	// 上一张票据已消费的时间步可能仍是当前步，等一个时间步保证验证码新鲜。
	next := time.Now().Add(mfacrypto.Period * time.Second)
	code2, err := mfacrypto.CodeAt(setup.Secret, next)
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.RedeemTicket(ctx, ticket2, code2)
	if err != nil {
		t.Fatalf("正确验证码应兑换成功: %v", err)
	}
	if got != accountID {
		t.Fatalf("兑换得到的账号 = %s, want %s", got, accountID)
	}
	if _, err := svc.RedeemTicket(ctx, ticket2, code2); !errors.Is(err, mfasvc.ErrTicketInvalid) {
		t.Fatalf("票据不可重复兑换，实际 %v", err)
	}
}

func TestDisableClearsBinding(t *testing.T) {
	gdb := setupMFA(t)
	ctx := context.Background()
	svc := newSvc(gdb)
	accountID := seedAccount(t, gdb, "mfa_disable")

	setup, err := svc.Setup(ctx, accountID, "mfa_disable")
	if err != nil {
		t.Fatal(err)
	}
	code, err := mfacrypto.CodeAt(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Enable(ctx, accountID, code); err != nil {
		t.Fatal(err)
	}
	if err := svc.Disable(ctx, accountID); err != nil {
		t.Fatal(err)
	}

	enabled, err := svc.IsEnabled(ctx, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("解绑后不应仍为已启用")
	}
	if err := svc.Verify(ctx, accountID, code); !errors.Is(err, mfasvc.ErrNotEnabled) {
		t.Fatalf("解绑后校验应返回 ErrNotEnabled，实际 %v", err)
	}
}

func TestSetupRejectsWhenAlreadyEnabled(t *testing.T) {
	gdb := setupMFA(t)
	ctx := context.Background()
	svc := newSvc(gdb)
	accountID := seedAccount(t, gdb, "mfa_dup")

	setup, err := svc.Setup(ctx, accountID, "mfa_dup")
	if err != nil {
		t.Fatal(err)
	}
	code, err := mfacrypto.CodeAt(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Enable(ctx, accountID, code); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Setup(ctx, accountID, "mfa_dup"); !errors.Is(err, mfasvc.ErrAlreadyEnabled) {
		t.Fatalf("已启用时重新绑定应被拒绝，实际 %v", err)
	}
}

func TestSetupRequiresConfiguredKey(t *testing.T) {
	gdb := setupMFA(t)
	ctx := context.Background()
	svc := mfasvc.NewService(gdb, nil, "MC 工具站测试")
	accountID := seedAccount(t, gdb, "mfa_nokey")

	if svc.Configured() {
		t.Fatal("未提供密钥时 Configured 应为 false")
	}
	if _, err := svc.Setup(ctx, accountID, "mfa_nokey"); !errors.Is(err, mfasvc.ErrNotConfigured) {
		t.Fatalf("未配置密钥时应返回 ErrNotConfigured，实际 %v", err)
	}
}
