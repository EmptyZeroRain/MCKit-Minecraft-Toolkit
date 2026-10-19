package contenttask_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"mckit/backend/internal/domain"
	db "mckit/backend/internal/repository"
	"mckit/backend/internal/search"
	"mckit/backend/internal/service/contenttask"

	"gorm.io/gorm"
)

// contentTaskSchema 本包集成测试专用 schema（隔离 cmd/api 集成测试对 public 的重建）。
const contentTaskSchema = "contenttask_it"

func withSearchPath(dsn, schema string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + schema
}

func setupContentTaskDB(t *testing.T) *gorm.DB {
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
	if err := adminDB.Exec("DROP SCHEMA IF EXISTS " + contentTaskSchema + " CASCADE").Error; err != nil {
		t.Fatalf("清理测试 schema 失败: %v", err)
	}
	if err := adminDB.Exec("CREATE SCHEMA " + contentTaskSchema).Error; err != nil {
		t.Fatalf("创建测试 schema 失败: %v", err)
	}
	if sqlDB, err := adminDB.DB(); err == nil {
		_ = sqlDB.Close()
	}

	gdb, err := db.Open(withSearchPath(url, contentTaskSchema))
	if err != nil {
		t.Fatalf("连接测试 schema 失败: %v", err)
	}
	if _, err := db.Migrate(ctx, gdb, "../../../migrations"); err != nil {
		t.Fatalf("执行迁移失败: %v", err)
	}
	return gdb
}

// fakeSearcher 可控的检索实现，用于验证「重新检测」。
type fakeSearcher struct {
	total int64
	err   error
}

func (f fakeSearcher) Search(context.Context, search.Filter) (*search.Result, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &search.Result{Total: f.total}, nil
}

func seedTerm(t *testing.T, gdb *gorm.DB, term string, hits, noResult int64) {
	t.Helper()
	if err := gdb.Create(&domain.SearchTerm{Term: term, Hits: hits, NoResultHits: noResult}).Error; err != nil {
		t.Fatalf("写入搜索词 %q 失败: %v", term, err)
	}
}

func seedAdmin(t *testing.T, gdb *gorm.DB, username string) *domain.Account {
	t.Helper()
	acc := &domain.Account{Username: username, PasswordHash: "x", Role: domain.RoleAdmin, IsActive: true}
	if err := gdb.Create(acc).Error; err != nil {
		t.Fatalf("创建后台账号失败: %v", err)
	}
	return acc
}

// TestContentTaskFlowIntegration 覆盖候选 → 创建 → 更新 → 重新检测 → 完成 → 重建。
func TestContentTaskFlowIntegration(t *testing.T) {
	gdb := setupContentTaskDB(t)
	ctx := context.Background()
	svc := contenttask.NewService(gdb, fakeSearcher{total: 0})
	admin := seedAdmin(t, gdb, "ct-admin")

	// 阈值以下的不进入候选；阈值以上的按无结果次数降序。
	seedTerm(t, gdb, "下界不足", 2, 2)
	seedTerm(t, gdb, "钻石矿分布", 10, 6)
	seedTerm(t, gdb, "远古残骸", 5, 3)

	candidates, err := svc.Candidates(ctx, 20)
	if err != nil {
		t.Fatalf("查询候选失败: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("期望 2 个候选（阈值 %d），实际 %d: %+v", domain.ContentTaskMinNoResultHits, len(candidates), candidates)
	}
	if candidates[0].Term != "钻石矿分布" || candidates[1].Term != "远古残骸" {
		t.Fatalf("候选应按无结果次数降序，实际 %+v", candidates)
	}

	// 创建任务：快照当前统计。
	task, err := svc.Create(ctx, &contenttask.CreateInput{
		Keyword:    "钻石矿分布",
		AssigneeID: admin.ID.String(),
		CreatedBy:  admin.ID.String(),
	})
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	if task.Status != domain.ContentTaskStatusOpen || task.NoResultHits != 6 || task.Hits != 10 {
		t.Fatalf("任务应快照统计且状态为 open，实际 %+v", task)
	}
	if task.SearchTermID == nil {
		t.Fatal("任务应关联搜索词统计行")
	}
	// 负责人展示名由服务端按账号解析（不信任客户端传入值）。
	if task.AssigneeID == nil || *task.AssigneeID != admin.ID || task.AssigneeName != "ct-admin" {
		t.Fatalf("负责人应解析为账号展示名，实际 id=%v name=%q", task.AssigneeID, task.AssigneeName)
	}

	// 同一关键词重复创建被拒。
	if _, err := svc.Create(ctx, &contenttask.CreateInput{Keyword: "钻石矿分布"}); !errors.Is(err, contenttask.ErrDuplicate) {
		t.Fatalf("重复创建应返回 ErrDuplicate，实际 %v", err)
	}

	// 候选列表标记出已有未完结任务的关键词。
	candidates, err = svc.Candidates(ctx, 20)
	if err != nil {
		t.Fatalf("查询候选失败: %v", err)
	}
	for _, c := range candidates {
		if c.Term == "钻石矿分布" && c.TaskID != task.ID.String() {
			t.Fatalf("候选应标记已有任务 ID，实际 %q", c.TaskID)
		}
	}

	// 非法状态 / 非法负责人被拒。
	bad := "unknown"
	if _, err := svc.Update(ctx, task.ID.String(), &contenttask.UpdateInput{Status: &bad}); !errors.Is(err, contenttask.ErrBadStatus) {
		t.Fatalf("非法状态应返回 ErrBadStatus，实际 %v", err)
	}
	badAssignee := "00000000-0000-0000-0000-000000000000"
	if _, err := svc.Update(ctx, task.ID.String(), &contenttask.UpdateInput{AssigneeID: &badAssignee}); !errors.Is(err, contenttask.ErrBadAssignee) {
		t.Fatalf("非法负责人应返回 ErrBadAssignee，实际 %v", err)
	}
	badRelated := "not-a-uuid"
	if _, err := svc.Update(ctx, task.ID.String(), &contenttask.UpdateInput{RelatedContentID: &badRelated}); !errors.Is(err, contenttask.ErrBadRelatedContent) {
		t.Fatalf("非法关联内容 ID 应返回 ErrBadRelatedContent，实际 %v", err)
	}

	// 关联内容 + 备注。
	ctype := domain.ContentTaskContentWiki
	cid := "11111111-1111-1111-1111-111111111111"
	note := "已在百科补充矿层分布"
	updated, err := svc.Update(ctx, task.ID.String(), &contenttask.UpdateInput{
		RelatedContentType: &ctype,
		RelatedContentID:   &cid,
		Note:               &note,
	})
	if err != nil {
		t.Fatalf("更新任务失败: %v", err)
	}
	if updated.RelatedContentType != ctype || updated.RelatedContentID == nil || updated.Note != note {
		t.Fatalf("关联内容 / 备注未写入，实际 %+v", updated)
	}

	// 重新检测：无结果时 HasResults=false。
	res, err := svc.Recheck(ctx, task.ID.String())
	if err != nil {
		t.Fatalf("重新检测失败: %v", err)
	}
	if res.HasResults || res.ResultCount != 0 {
		t.Fatalf("期望无结果，实际 %+v", res)
	}

	// 有结果时 HasResults=true（用于提示可标记完成）。
	withResults := contenttask.NewService(gdb, fakeSearcher{total: 3})
	res, err = withResults.Recheck(ctx, task.ID.String())
	if err != nil {
		t.Fatalf("重新检测失败: %v", err)
	}
	if !res.HasResults || res.ResultCount != 3 {
		t.Fatalf("期望已有结果，实际 %+v", res)
	}

	// 标记完成。
	done := domain.ContentTaskStatusDone
	if _, err := svc.Update(ctx, task.ID.String(), &contenttask.UpdateInput{Status: &done}); err != nil {
		t.Fatalf("标记完成失败: %v", err)
	}

	// 完成后可再次创建同一关键词（部分唯一索引仅约束未完结任务）。
	again, err := svc.Create(ctx, &contenttask.CreateInput{Keyword: "钻石矿分布"})
	if err != nil {
		t.Fatalf("完成后重建任务失败: %v", err)
	}
	if again.ID == task.ID {
		t.Fatal("重建任务应生成新记录")
	}

	// 列表按状态筛选。
	items, total, err := svc.List(ctx, contenttask.ListFilter{Status: domain.ContentTaskStatusOpen})
	if err != nil {
		t.Fatalf("查询任务列表失败: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != again.ID {
		t.Fatalf("open 状态应只有重建任务，实际 total=%d items=%+v", total, items)
	}

	// 删除。
	if err := svc.Delete(ctx, again.ID.String()); err != nil {
		t.Fatalf("删除任务失败: %v", err)
	}
	if err := svc.Delete(ctx, again.ID.String()); !errors.Is(err, contenttask.ErrNotFound) {
		t.Fatalf("重复删除应返回 ErrNotFound，实际 %v", err)
	}
}

// TestContentTaskKeywordNormalizationIntegration 验证关键词按搜索词口径归一化。
func TestContentTaskKeywordNormalizationIntegration(t *testing.T) {
	gdb := setupContentTaskDB(t)
	ctx := context.Background()
	svc := contenttask.NewService(gdb, nil)

	task, err := svc.Create(ctx, &contenttask.CreateInput{Keyword: "  钻石  矿分布  "})
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	if task.Keyword != "钻石 矿分布" {
		t.Fatalf("关键词应折叠空白并转小写，实际 %q", task.Keyword)
	}
	if _, err := svc.Create(ctx, &contenttask.CreateInput{Keyword: "   "}); !errors.Is(err, contenttask.ErrBadKeyword) {
		t.Fatalf("空关键词应返回 ErrBadKeyword，实际 %v", err)
	}

	// 未配置检索服务时重新检测明确报错，而不是静默返回 0。
	if _, err := svc.Recheck(ctx, task.ID.String()); !errors.Is(err, contenttask.ErrSearcherUnavailable) {
		t.Fatalf("未配置检索服务应返回 ErrSearcherUnavailable，实际 %v", err)
	}
}
