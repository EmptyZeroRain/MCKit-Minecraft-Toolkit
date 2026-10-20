package accountadmin

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"mckit/backend/internal/domain"
	db "mckit/backend/internal/repository"

	"gorm.io/gorm"
)

// accountAdminSchema 统一账号集成测试专用 schema。
//
// go test 默认并行跑包，cmd/api 的集成测试会重建 public schema；这里用独立
// schema 避免互相影响（与 adminaccount 包的测试策略一致）。
const accountAdminSchema = "accountadmin_it"

func setupAccountAdminDB(t *testing.T) *gorm.DB {
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
	if err := adminDB.Exec("DROP SCHEMA IF EXISTS " + accountAdminSchema + " CASCADE").Error; err != nil {
		t.Fatalf("清理测试 schema 失败: %v", err)
	}
	if err := adminDB.Exec("CREATE SCHEMA " + accountAdminSchema).Error; err != nil {
		t.Fatalf("创建测试 schema 失败: %v", err)
	}
	if sqlDB, err := adminDB.DB(); err == nil {
		_ = sqlDB.Close()
	}

	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	gdb, err := db.Open(url + sep + "search_path=" + accountAdminSchema)
	if err != nil {
		t.Fatalf("连接测试 schema 失败: %v", err)
	}
	if _, err := db.Migrate(ctx, gdb, "../../../migrations"); err != nil {
		t.Fatalf("执行迁移失败: %v", err)
	}
	return gdb
}

// TestRoleLifecycleIntegration 验证普通用户提升 / 降级与密码策略标记。
func TestRoleLifecycleIntegration(t *testing.T) {
	gdb := setupAccountAdminDB(t)
	ctx := context.Background()
	svc := NewService(gdb)

	created, err := svc.Create(ctx, CreateInput{
		Username: "it-promote", Password: "user-pass-123", Role: domain.RoleUser,
	})
	if err != nil {
		t.Fatalf("创建普通用户失败: %v", err)
	}

	editor := "editor"
	updated, err := svc.Update(ctx, created.ID, UpdateInput{Role: &editor})
	if err != nil {
		t.Fatalf("提升为 editor 失败: %v", err)
	}
	if updated.Role != editor {
		t.Fatalf("提升后角色=%q，期望 editor", updated.Role)
	}
	var acc domain.Account
	if err := gdb.Where("id = ?", created.ID).First(&acc).Error; err != nil {
		t.Fatalf("回查账号失败: %v", err)
	}
	if !acc.MustChangePassword {
		t.Fatal("提升为后台角色后应要求下次登录改密")
	}

	userRole := domain.RoleUser
	updated, err = svc.Update(ctx, created.ID, UpdateInput{Role: &userRole})
	if err != nil {
		t.Fatalf("降级为普通用户失败: %v", err)
	}
	if updated.Role != domain.RoleUser {
		t.Fatalf("降级后角色=%q，期望 user", updated.Role)
	}
}

// TestLastAdminGuardIntegration 验证停用 / 降级 / 删除都不会清空启用 admin。
func TestLastAdminGuardIntegration(t *testing.T) {
	gdb := setupAccountAdminDB(t)
	ctx := context.Background()
	svc := NewService(gdb)

	a1, err := svc.Create(ctx, CreateInput{Username: "it-admin-a", Password: "integration-pass-123", Role: domain.RoleAdmin})
	if err != nil {
		t.Fatalf("创建 admin A 失败: %v", err)
	}
	a2, err := svc.Create(ctx, CreateInput{Username: "it-admin-b", Password: "integration-pass-123", Role: domain.RoleAdmin})
	if err != nil {
		t.Fatalf("创建 admin B 失败: %v", err)
	}

	disabled := false
	if _, err := svc.Update(ctx, a1.ID, UpdateInput{IsActive: &disabled}); err != nil {
		t.Fatalf("停用 admin A 失败: %v", err)
	}

	if _, err := svc.Update(ctx, a2.ID, UpdateInput{IsActive: &disabled}); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("停用最后一名 admin 期望 ErrLastAdmin，实际 %v", err)
	}
	userRole := domain.RoleUser
	if _, err := svc.Update(ctx, a2.ID, UpdateInput{Role: &userRole}); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("降级最后一名 admin 期望 ErrLastAdmin，实际 %v", err)
	}
	if err := svc.Delete(ctx, a2.ID); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("删除最后一名 admin 期望 ErrLastAdmin，实际 %v", err)
	}

	enabled := true
	if _, err := svc.Update(ctx, a1.ID, UpdateInput{IsActive: &enabled}); err != nil {
		t.Fatalf("重新启用 admin A 失败: %v", err)
	}
	if err := svc.Delete(ctx, a2.ID); err != nil {
		t.Fatalf("保留一名启用 admin 后应允许删除，实际 %v", err)
	}
}
