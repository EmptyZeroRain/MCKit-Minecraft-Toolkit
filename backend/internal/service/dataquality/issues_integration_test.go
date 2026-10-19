package dataquality_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	db "mckit/backend/internal/repository"
	"mckit/backend/internal/service/dataquality"
)

const dqSchema = "dataquality_it"

func withSearchPath(dsn, schema string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + schema
}

func setupDQDB(t *testing.T) *gorm.DB {
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
	if err := adminDB.Exec("DROP SCHEMA IF EXISTS " + dqSchema + " CASCADE").Error; err != nil {
		t.Fatalf("清理测试 schema 失败: %v", err)
	}
	if err := adminDB.Exec("CREATE SCHEMA " + dqSchema).Error; err != nil {
		t.Fatalf("创建测试 schema 失败: %v", err)
	}
	if sqlDB, err := adminDB.DB(); err == nil {
		_ = sqlDB.Close()
	}

	gdb, err := db.Open(withSearchPath(url, dqSchema))
	if err != nil {
		t.Fatalf("连接测试 schema 失败: %v", err)
	}
	if _, err := db.Migrate(ctx, gdb, "../../../migrations"); err != nil {
		t.Fatalf("执行迁移失败: %v", err)
	}
	return gdb
}

func seedAdmin(t *testing.T, gdb *gorm.DB, username, nickname string) domain.Account {
	t.Helper()
	acc := domain.Account{
		Username:     username,
		PasswordHash: "x",
		Nickname:     nickname,
		Role:         domain.RoleAdmin,
		IsActive:     true,
	}
	if err := gdb.Create(&acc).Error; err != nil {
		t.Fatalf("创建管理员失败: %v", err)
	}
	return acc
}

func TestAssigneeSnapshotAndReopen(t *testing.T) {
	gdb := setupDQDB(t)
	ctx := context.Background()
	svc := dataquality.NewService(gdb)
	acc := seedAdmin(t, gdb, "dq-admin", "质量负责人")

	list, err := svc.Assignees(ctx)
	if err != nil {
		t.Fatalf("查询负责人失败: %v", err)
	}
	if len(list) != 1 || list[0].Name != "质量负责人" {
		t.Fatalf("负责人列表异常: %+v", list)
	}

	// 指派：状态与负责人同时落地，并快照当时命中条数。
	issue, err := svc.UpdateIssue(ctx, dataquality.UpdateIssueInput{
		CheckKey:      "items_invalid_versions",
		Status:        domain.QualityIssueAssigned,
		AssigneeID:    &acc.ID,
		Note:          "已通知数据组",
		SnapshotCount: 12,
		Operator:      "operator",
	})
	if err != nil {
		t.Fatalf("更新工单失败: %v", err)
	}
	if issue.Status != domain.QualityIssueAssigned || issue.AssigneeName != "质量负责人" {
		t.Fatalf("指派结果异常: %+v", issue)
	}
	if issue.SnapshotCount != 12 {
		t.Fatalf("快照条数应为 12，实际 %d", issue.SnapshotCount)
	}

	// 解决：记录解决时间。
	issue, err = svc.UpdateIssue(ctx, dataquality.UpdateIssueInput{
		CheckKey:      "items_invalid_versions",
		Status:        domain.QualityIssueResolved,
		AssigneeID:    &acc.ID,
		Note:          "已修正",
		SnapshotCount: 0,
		Operator:      "operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.ResolvedAt == nil {
		t.Fatal("已解决工单应记录 resolved_at")
	}

	// 重新打开：清空解决时间，避免残留。
	issue, err = svc.UpdateIssue(ctx, dataquality.UpdateIssueInput{
		CheckKey:      "items_invalid_versions",
		Status:        domain.QualityIssueOpen,
		SnapshotCount: 3,
		Operator:      "operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Status != domain.QualityIssueOpen || issue.ResolvedAt != nil {
		t.Fatalf("重新打开后状态异常: %+v", issue)
	}
	if issue.AssigneeID != nil || issue.AssigneeName != "" {
		t.Fatalf("未传负责人时应清空指派: %+v", issue)
	}

	// 同一 check_key 只应有一条工单。
	var count int64
	if err := gdb.Model(&domain.DataQualityIssue{}).
		Where("check_key = ?", "items_invalid_versions").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("同一检查项应只有一条工单，实际 %d", count)
	}
}

func TestUpdateIssueValidation(t *testing.T) {
	gdb := setupDQDB(t)
	ctx := context.Background()
	svc := dataquality.NewService(gdb)
	seedAdmin(t, gdb, "dq-admin2", "")
	// is_active 带 default:true，直接 Create(false) 会被默认值覆盖，需显式更新。
	inactive := domain.Account{Username: "dq-off", PasswordHash: "x", Role: domain.RoleAdmin}
	if err := gdb.Create(&inactive).Error; err != nil {
		t.Fatal(err)
	}
	if err := gdb.Model(&inactive).Update("is_active", false).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := svc.UpdateIssue(ctx, dataquality.UpdateIssueInput{
		CheckKey: "x", Status: "bogus",
	}); !errors.Is(err, dataquality.ErrBadIssue) {
		t.Fatalf("非法状态应返回 ErrBadIssue，实际 %v", err)
	}
	if _, err := svc.UpdateIssue(ctx, dataquality.UpdateIssueInput{
		CheckKey: "", Status: domain.QualityIssueOpen,
	}); !errors.Is(err, dataquality.ErrBadIssue) {
		t.Fatalf("空 check_key 应返回 ErrBadIssue，实际 %v", err)
	}
	if _, err := svc.UpdateIssue(ctx, dataquality.UpdateIssueInput{
		CheckKey: "x", Status: domain.QualityIssueAssigned, AssigneeID: &inactive.ID,
	}); !errors.Is(err, dataquality.ErrBadIssue) {
		t.Fatalf("停用账号不应可被指派，实际 %v", err)
	}

	// assigned 但未指定负责人 → 归一化为 open。
	issue, err := svc.UpdateIssue(ctx, dataquality.UpdateIssueInput{
		CheckKey: "y", Status: domain.QualityIssueAssigned,
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Status != domain.QualityIssueOpen {
		t.Fatalf("无负责人的指派应归一化为 open，实际 %s", issue.Status)
	}
}
