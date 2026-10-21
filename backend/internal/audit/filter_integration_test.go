package audit_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
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

// TestLoginHistoryScopesToAccount 「登录历史」只能看到本人的登录：
//   - 本人的成功登录按 actor_admin_id 归集；
//   - 失败登录没有 actor，只能按 after_summary.username 归集（撞库尝试也要可见）；
//   - 其它账号的登录、非登录动作一律不出现。
func TestLoginHistoryScopesToAccount(t *testing.T) {
	gdb := setupAuditDB(t)
	svc := audit.NewService(gdb)

	me := uuid.MustParse("11111111-1111-1111-1111-111111111111").String()
	other := uuid.MustParse("22222222-2222-2222-2222-222222222222").String()

	// audit_logs.actor_admin_id 有外键（0022 起指向 accounts），先准备两个管理员账号。
	for _, a := range []struct{ id, username string }{{me, "it-alice"}, {other, "it-bob"}} {
		if err := gdb.Exec(
			"INSERT INTO accounts (id, username, password_hash, role) VALUES (?, ?, ?, 'admin')",
			a.id, a.username, "x",
		).Error; err != nil {
			t.Fatalf("准备管理员失败: %v", err)
		}
	}

	seed := func(actor, action, result, ip, after string) {
		t.Helper()
		row := domain.AuditLog{
			Action:        action,
			ObjectType:    "admin",
			Result:        result,
			SourceIP:      ip,
			BeforeSummary: "{}",
			AfterSummary:  after,
		}
		if actor != "" {
			id := uuid.MustParse(actor)
			row.ActorAdminID = &id
		}
		if err := gdb.Create(&row).Error; err != nil {
			t.Fatalf("写入登录审计失败: %v", err)
		}
	}

	seed(me, "admin.login", domain.AuditSuccess, "198.51.100.10", `{"user_agent":"UA-OK"}`)
	seed("", "admin.login", domain.AuditFailure, "198.51.100.11", `{"username":"alice","user_agent":"UA-FAIL"}`)
	seed("", "admin.login", domain.AuditFailure, "198.51.100.12", `{"username":"bob","user_agent":"UA-OTHER"}`)
	seed(other, "admin.login", domain.AuditSuccess, "198.51.100.13", `{}`)
	seed(me, "resource.update", domain.AuditSuccess, "198.51.100.14", `{}`)

	items, total, err := svc.LoginHistory(context.Background(), me, "alice", 1, audit.LoginHistoryMax)
	if err != nil {
		t.Fatalf("查询登录历史失败: %v", err)
	}
	if total != 2 {
		t.Fatalf("期望 2 条（本人成功 + 针对本人的失败），实际 total=%d", total)
	}
	if len(items) != 2 {
		t.Fatalf("期望 2 条（本人成功 + 针对本人的失败），实际 %d 条: %+v", len(items), items)
	}
	got := map[string]string{}
	for _, it := range items {
		ua, _ := it.AfterSummary["user_agent"].(string)
		got[it.SourceIP] = ua
	}
	if got["198.51.100.10"] != "UA-OK" || got["198.51.100.11"] != "UA-FAIL" {
		t.Fatalf("登录历史内容不符: %+v", got)
	}

	// 分页：每页 1 条时第 1 / 2 页应返回不同记录。
	p1, total1, err := svc.LoginHistory(context.Background(), me, "alice", 1, 1)
	if err != nil {
		t.Fatalf("查询第 1 页失败: %v", err)
	}
	p2, _, err := svc.LoginHistory(context.Background(), me, "alice", 2, 1)
	if err != nil {
		t.Fatalf("查询第 2 页失败: %v", err)
	}
	if total1 != 2 || len(p1) != 1 || len(p2) != 1 || p1[0].ID == p2[0].ID {
		t.Fatalf("分页结果不符：total=%d p1=%+v p2=%+v", total1, p1, p2)
	}
}
