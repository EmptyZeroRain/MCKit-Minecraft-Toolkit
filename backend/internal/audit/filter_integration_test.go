package audit_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"gorm.io/gorm"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/domain"
	db "mckit/backend/internal/repository"
)

const auditSchema = "audit_it"

func withSearchPath(dsn, schema string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + schema
}

func setupAuditDB(t *testing.T) *gorm.DB {
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
	if err := adminDB.Exec("DROP SCHEMA IF EXISTS " + auditSchema + " CASCADE").Error; err != nil {
		t.Fatalf("清理测试 schema 失败: %v", err)
	}
	if err := adminDB.Exec("CREATE SCHEMA " + auditSchema).Error; err != nil {
		t.Fatalf("创建测试 schema 失败: %v", err)
	}
	if sqlDB, err := adminDB.DB(); err == nil {
		_ = sqlDB.Close()
	}

	gdb, err := db.Open(withSearchPath(url, auditSchema))
	if err != nil {
		t.Fatalf("连接测试 schema 失败: %v", err)
	}
	if _, err := db.Migrate(ctx, gdb, "../../migrations"); err != nil {
		t.Fatalf("执行迁移失败: %v", err)
	}
	return gdb
}

func seedLog(t *testing.T, gdb *gorm.DB, action, objectType, result, ip string) {
	t.Helper()
	// before_summary / after_summary 是 jsonb 列，空串会导致 22P02，需给合法 JSON。
	row := domain.AuditLog{
		Action:        action,
		ObjectType:    objectType,
		Result:        result,
		SourceIP:      ip,
		BeforeSummary: "{}",
		AfterSummary:  "{}",
	}
	if err := gdb.Create(&row).Error; err != nil {
		t.Fatalf("写入审计日志失败: %v", err)
	}
}

// TestListFiltersActuallyApply 回归 gj.md 阶段三 ⑨：object_type / result 曾被忽略。
func TestListFiltersActuallyApply(t *testing.T) {
	gdb := setupAuditDB(t)
	svc := audit.NewService(gdb)

	seedLog(t, gdb, "admin.login", "admin", domain.AuditFailure, "198.51.100.1")
	seedLog(t, gdb, "admin.login", "admin", domain.AuditSuccess, "198.51.100.2")
	seedLog(t, gdb, "resource.update", "resource", domain.AuditSuccess, "198.51.100.3")
	seedLog(t, gdb, "admin.mfa.enable", "admin", domain.AuditSuccess, "198.51.100.4")

	tests := []struct {
		name   string
		filter audit.ListFilter
		want   int
	}{
		{"全部", audit.ListFilter{}, 4},
		{"按对象类型", audit.ListFilter{ObjectType: "resource"}, 1},
		{"按结果", audit.ListFilter{Result: domain.AuditFailure}, 1},
		{"对象+结果", audit.ListFilter{ObjectType: "admin", Result: domain.AuditSuccess}, 2},
		{"按来源 IP", audit.ListFilter{SourceIP: "198.51.100.4"}, 1},
		{"安全事件前缀", audit.ListFilter{ActionPrefixes: audit.SecurityActionPrefixes}, 3},
		{
			"安全事件+失败",
			audit.ListFilter{ActionPrefixes: audit.SecurityActionPrefixes, Result: domain.AuditFailure},
			1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			items, total, err := svc.List(tc.filter)
			if err != nil {
				t.Fatalf("查询失败: %v", err)
			}
			if int(total) != tc.want || len(items) != tc.want {
				t.Fatalf("期望 %d 条，实际 total=%d items=%d", tc.want, total, len(items))
			}
		})
	}
}

func TestExportHonoursFiltersAndLimit(t *testing.T) {
	gdb := setupAuditDB(t)
	svc := audit.NewService(gdb)
	for i := 0; i < 5; i++ {
		seedLog(t, gdb, "resource.update", "resource", domain.AuditSuccess, "198.51.100.9")
	}
	seedLog(t, gdb, "admin.login", "admin", domain.AuditFailure, "198.51.100.9")

	items, truncated, err := svc.Export(context.Background(), audit.ListFilter{ObjectType: "resource"})
	if err != nil {
		t.Fatalf("导出失败: %v", err)
	}
	if len(items) != 5 || truncated {
		t.Fatalf("导出条数异常: len=%d truncated=%v", len(items), truncated)
	}
}
