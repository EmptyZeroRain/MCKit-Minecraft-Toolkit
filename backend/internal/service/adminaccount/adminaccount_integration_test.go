package adminaccount

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"mckit/backend/internal/domain"
	db "mckit/backend/internal/repository"

	"gorm.io/gorm"
)

// adminAccountSchema 本包集成测试专用 schema。
//
// 与 linkhealth 集成测试同理：go test 默认并行跑包，cmd/api 的集成测试会执行
// `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`，会波及共用同一测试库的
// 其他包。因此本包把对象建在独立 schema 中隔离。
const adminAccountSchema = "adminaccount_it"

// withSearchPath 在 DSN 上追加 search_path 运行时参数（pgx 会把未知参数作为运行时参数下发）。
func withSearchPath(dsn, schema string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + schema
}

// setupAdminAccountDB 准备独立 schema 并执行迁移，返回可用连接。
func setupAdminAccountDB(t *testing.T) *gorm.DB {
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
	if err := adminDB.Exec("DROP SCHEMA IF EXISTS " + adminAccountSchema + " CASCADE").Error; err != nil {
		t.Fatalf("清理测试 schema 失败: %v", err)
	}
	if err := adminDB.Exec("CREATE SCHEMA " + adminAccountSchema).Error; err != nil {
		t.Fatalf("创建测试 schema 失败: %v", err)
	}
	if sqlDB, err := adminDB.DB(); err == nil {
		_ = sqlDB.Close()
	}

	gdb, err := db.Open(withSearchPath(url, adminAccountSchema))
	if err != nil {
		t.Fatalf("连接测试 schema 失败: %v", err)
	}
	if _, err := db.Migrate(ctx, gdb, "../../../migrations"); err != nil {
		t.Fatalf("执行迁移失败: %v", err)
	}
	return gdb
}

// TestLastAdminGuardConcurrentDisableIntegration 验证并发停用不会清空所有 admin。
//
// 场景：存在 2 名启用 admin，两个请求并发停用不同的 admin。
// 期望：恰好 1 个成功、1 个被 ErrLastAdmin 拒绝，最终仍保留 1 名启用 admin。
func TestLastAdminGuardConcurrentDisableIntegration(t *testing.T) {
	gdb := setupAdminAccountDB(t)
	ctx := context.Background()
	svc := NewService(gdb)

	a1, err := svc.Create(ctx, CreateInput{Username: "it-guard-a", Password: "integration-pass-123", Role: domain.RoleAdmin})
	if err != nil {
		t.Fatalf("创建管理员 A 失败: %v", err)
	}
	a2, err := svc.Create(ctx, CreateInput{Username: "it-guard-b", Password: "integration-pass-123", Role: domain.RoleAdmin})
	if err != nil {
		t.Fatalf("创建管理员 B 失败: %v", err)
	}

	ids := []string{a1.ID.String(), a2.ID.String()}
	results := make([]error, len(ids))
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			inactive := false
			_, e := svc.Update(ctx, ids[i], UpdateInput{IsActive: &inactive})
			results[i] = e
		}(i)
	}
	wg.Wait()

	success, rejected := 0, 0
	for _, e := range results {
		switch {
		case e == nil:
			success++
		case errors.Is(e, ErrLastAdmin):
			rejected++
		default:
			t.Fatalf("出现非预期错误: %v", e)
		}
	}
	if success != 1 || rejected != 1 {
		t.Fatalf("期望 1 成功 1 被拒，实际 success=%d rejected=%d", success, rejected)
	}

	var active int64
	if err := gdb.Model(&domain.Admin{}).
		Where("role = ? AND is_active = true", domain.RoleAdmin).Count(&active).Error; err != nil {
		t.Fatalf("统计启用 admin 失败: %v", err)
	}
	if active != 1 {
		t.Fatalf("并发停用后应保留 1 名启用 admin，实际 %d", active)
	}
}

// TestLastAdminGuardConcurrentDeleteIntegration 验证并发删除不会清空所有 admin。
func TestLastAdminGuardConcurrentDeleteIntegration(t *testing.T) {
	gdb := setupAdminAccountDB(t)
	ctx := context.Background()
	svc := NewService(gdb)

	a1, err := svc.Create(ctx, CreateInput{Username: "it-del-a", Password: "integration-pass-123", Role: domain.RoleAdmin})
	if err != nil {
		t.Fatalf("创建管理员 A 失败: %v", err)
	}
	a2, err := svc.Create(ctx, CreateInput{Username: "it-del-b", Password: "integration-pass-123", Role: domain.RoleAdmin})
	if err != nil {
		t.Fatalf("创建管理员 B 失败: %v", err)
	}

	ids := []string{a1.ID.String(), a2.ID.String()}
	results := make([]error, len(ids))
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = svc.Delete(ctx, ids[i])
		}(i)
	}
	wg.Wait()

	success, rejected := 0, 0
	for _, e := range results {
		switch {
		case e == nil:
			success++
		case errors.Is(e, ErrLastAdmin):
			rejected++
		default:
			t.Fatalf("出现非预期错误: %v", e)
		}
	}
	if success != 1 || rejected != 1 {
		t.Fatalf("期望 1 成功 1 被拒，实际 success=%d rejected=%d", success, rejected)
	}

	var active int64
	if err := gdb.Model(&domain.Admin{}).
		Where("role = ? AND is_active = true", domain.RoleAdmin).Count(&active).Error; err != nil {
		t.Fatalf("统计启用 admin 失败: %v", err)
	}
	if active != 1 {
		t.Fatalf("并发删除后应保留 1 名启用 admin，实际 %d", active)
	}
}
