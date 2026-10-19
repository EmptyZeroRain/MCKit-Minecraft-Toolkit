package alert_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	db "mckit/backend/internal/repository"
	"mckit/backend/internal/service/alert"
)

const alertSchema = "alert_it"

func withSearchPath(dsn, schema string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + schema
}

func setupAlertDB(t *testing.T) *gorm.DB {
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
	if err := adminDB.Exec("DROP SCHEMA IF EXISTS " + alertSchema + " CASCADE").Error; err != nil {
		t.Fatalf("清理测试 schema 失败: %v", err)
	}
	if err := adminDB.Exec("CREATE SCHEMA " + alertSchema).Error; err != nil {
		t.Fatalf("创建测试 schema 失败: %v", err)
	}
	if sqlDB, err := adminDB.DB(); err == nil {
		_ = sqlDB.Close()
	}

	gdb, err := db.Open(withSearchPath(url, alertSchema))
	if err != nil {
		t.Fatalf("连接测试 schema 失败: %v", err)
	}
	if _, err := db.Migrate(ctx, gdb, "../../../migrations"); err != nil {
		t.Fatalf("执行迁移失败: %v", err)
	}
	return gdb
}

// seedResource 建一条最小可用资源（链接检查表对 resources 有外键）。
func seedResource(t *testing.T, gdb *gorm.DB) string {
	t.Helper()
	id := uuid.NewString()
	if err := gdb.Exec(`INSERT INTO resources (id, slug, name, summary, resource_type, source_url)
		VALUES (?, ?, ?, '', 'mod', 'https://example.com/source')`,
		id, "alert-test-"+id[:8], "告警测试资源").Error; err != nil {
		t.Fatalf("创建资源失败: %v", err)
	}
	return id
}

// seedLinkChecks 写入若干条链接复查结果（broken / ok）。
func seedLinkChecks(t *testing.T, gdb *gorm.DB, broken, ok int) {
	t.Helper()
	for i := 0; i < broken; i++ {
		c := domain.LinkCheck{
			ID:          uuid.NewString(),
			ResourceID:  seedResource(t, gdb),
			URL:         "https://example.com/broken",
			Status:      domain.LinkStatusBroken,
			ReviewState: domain.ReviewPending,
		}
		if err := gdb.Create(&c).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < ok; i++ {
		c := domain.LinkCheck{
			ID:          uuid.NewString(),
			ResourceID:  seedResource(t, gdb),
			URL:         "https://example.com/ok",
			Status:      domain.LinkStatusOK,
			ReviewState: domain.ReviewPending,
		}
		if err := gdb.Create(&c).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestEvaluateTriggersDedupesAndAutoCloses(t *testing.T) {
	gdb := setupAlertDB(t)
	ctx := context.Background()
	svc := alert.NewService(gdb)

	// 无异常数据：不产生告警。
	sum, err := svc.Evaluate(ctx)
	if err != nil {
		t.Fatalf("评估失败: %v", err)
	}
	if sum.Triggered != 0 || sum.AutoClosed != 0 {
		t.Fatalf("空库不应有告警: %+v", sum)
	}

	// 10 条已复查链接中 6 条失效且未确认 → 触发。
	seedLinkChecks(t, gdb, 6, 4)
	sum, err = svc.Evaluate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Triggered != 1 {
		t.Fatalf("应触发 1 条告警，实际 %+v", sum)
	}

	rows, total, err := svc.List(ctx, alert.ListFilter{Status: domain.AlertStatusOpen})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || rows[0].RuleKey != "linkhealth.broken_pending" {
		t.Fatalf("告警落库异常: total=%d rows=%+v", total, rows)
	}
	if rows[0].Metric == nil {
		t.Error("告警应带指标快照")
	}

	// 再次评估：刷新而不是新增（避免刷屏）。
	sum, err = svc.Evaluate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Refreshed != 1 || sum.Triggered != 0 {
		t.Fatalf("重复评估应刷新已有告警: %+v", sum)
	}
	_, total, _ = svc.List(ctx, alert.ListFilter{Status: domain.AlertStatusOpen})
	if total != 1 {
		t.Fatalf("同一规则不应产生重复告警，实际 %d 条", total)
	}

	// 指标恢复 → 自动关闭。
	if err := gdb.Model(&domain.LinkCheck{}).
		Where("status = ?", domain.LinkStatusBroken).
		Update("review_state", domain.ReviewConfirmedBroken).Error; err != nil {
		t.Fatal(err)
	}
	sum, err = svc.Evaluate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sum.AutoClosed != 1 {
		t.Fatalf("指标恢复后应自动关闭: %+v", sum)
	}

	// 关闭后保留历史。
	_, total, _ = svc.List(ctx, alert.ListFilter{Status: domain.AlertStatusResolved})
	if total != 1 {
		t.Fatalf("已关闭告警应保留为历史，实际 %d 条", total)
	}
}

func TestAckAndResolve(t *testing.T) {
	gdb := setupAlertDB(t)
	ctx := context.Background()
	svc := alert.NewService(gdb)

	seedLinkChecks(t, gdb, 10, 0)
	if _, err := svc.Evaluate(ctx); err != nil {
		t.Fatal(err)
	}
	rows, _, err := svc.List(ctx, alert.ListFilter{Status: domain.AlertStatusOpen})
	if err != nil || len(rows) != 1 {
		t.Fatalf("准备告警失败: %v rows=%d", err, len(rows))
	}
	id := rows[0].ID

	acked, err := svc.Ack(ctx, id, "ops")
	if err != nil {
		t.Fatalf("确认失败: %v", err)
	}
	if acked.Status != domain.AlertStatusAcked || acked.AckedBy != "ops" || acked.AckedAt == nil {
		t.Fatalf("确认结果异常: %+v", acked)
	}
	// 已确认的告警不能再次确认。
	if _, err := svc.Ack(ctx, id, "ops"); !errors.Is(err, alert.ErrNotOpen) {
		t.Fatalf("重复确认应返回 ErrNotOpen，实际 %v", err)
	}

	resolved, err := svc.Resolve(ctx, id, "ops", "已联系资源作者")
	if err != nil {
		t.Fatalf("关闭失败: %v", err)
	}
	if resolved.Status != domain.AlertStatusResolved || resolved.ResolvedAt == nil {
		t.Fatalf("关闭结果异常: %+v", resolved)
	}
	if !strings.Contains(resolved.Detail, "已联系资源作者") {
		t.Fatalf("关闭原因未记录: %s", resolved.Detail)
	}
	if _, err := svc.Resolve(ctx, id, "ops", ""); !errors.Is(err, alert.ErrNotOpen) {
		t.Fatalf("重复关闭应返回 ErrNotOpen，实际 %v", err)
	}

	sum, err := svc.Summary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Open != 0 || sum.Acked != 0 {
		t.Fatalf("关闭后未结告警应为 0: %+v", sum)
	}

	if _, err := svc.Ack(ctx, uuid.New(), "ops"); !errors.Is(err, alert.ErrNotFound) {
		t.Fatalf("不存在的告警应返回 ErrNotFound，实际 %v", err)
	}
}
