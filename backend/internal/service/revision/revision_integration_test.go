package revision_test

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"mckit/backend/internal/domain"
	db "mckit/backend/internal/repository"
	"mckit/backend/internal/service/content"
	"mckit/backend/internal/service/resource"
	"mckit/backend/internal/service/revision"

	"gorm.io/gorm"
)

// revisionSchema 本包集成测试专用 schema。
//
// go test 默认并行跑包，cmd/api 的集成测试会 `DROP SCHEMA public CASCADE`，
// 会波及共用同一测试库的其他包，因此把对象建在独立 schema 中隔离。
const revisionSchema = "revision_it"

func withSearchPath(dsn, schema string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + schema
}

func setupRevisionDB(t *testing.T) *gorm.DB {
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
	if err := adminDB.Exec("DROP SCHEMA IF EXISTS " + revisionSchema + " CASCADE").Error; err != nil {
		t.Fatalf("清理测试 schema 失败: %v", err)
	}
	if err := adminDB.Exec("CREATE SCHEMA " + revisionSchema).Error; err != nil {
		t.Fatalf("创建测试 schema 失败: %v", err)
	}
	if sqlDB, err := adminDB.DB(); err == nil {
		_ = sqlDB.Close()
	}

	gdb, err := db.Open(withSearchPath(url, revisionSchema))
	if err != nil {
		t.Fatalf("连接测试 schema 失败: %v", err)
	}
	if _, err := db.Migrate(ctx, gdb, "../../../migrations"); err != nil {
		t.Fatalf("执行迁移失败: %v", err)
	}
	return gdb
}

// newHarness 组装内容服务 + 历史版本服务（还原器按内容类型注册）。
type harness struct {
	db      *gorm.DB
	res     *resource.Service
	content *content.Service
	rev     *revision.Service
	actorID string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	gdb := setupRevisionDB(t)
	resSvc := resource.NewService(gdb)
	contentSvc := content.NewService(gdb)
	revSvc := revision.NewService(gdb)
	revSvc.Register(domain.RevisionContentResource, resSvc)
	revSvc.Register(domain.RevisionContentTool, contentSvc)
	revSvc.Register(domain.RevisionContentWiki, contentSvc)

	// 内容表的 created_by / updated_by 外键指向 accounts，需要真实操作者账号。
	actor := &domain.Account{
		Username:     "it-rev-actor-" + uuid.New().String()[:8],
		PasswordHash: "not-a-real-hash",
		Role:         domain.RoleAdmin,
		IsActive:     true,
	}
	if err := gdb.Create(actor).Error; err != nil {
		t.Fatalf("创建测试账号失败: %v", err)
	}
	return &harness{db: gdb, res: resSvc, content: contentSvc, rev: revSvc, actorID: actor.ID.String()}
}

// TestResourceRevisionFlowIntegration 覆盖资源「创建 → 更新 → 差异 → 回滚」全链路。
func TestResourceRevisionFlowIntegration(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	res, err := h.res.Create(ctx, &resource.CreateInput{
		Slug:              "it-rev-resource",
		Name:              "原始名称",
		Summary:           "原始摘要",
		ResourceType:      domain.ResourceTypeMod,
		SourceURL:         "https://example.com/mod",
		Status:            domain.StatusPublished,
		Tags:              []string{"alpha", "beta"},
		CreatedBy:         h.actorID,
		ActorName:         "it-admin",
		SupportedVersions: []string{"1.20.1"},
	})
	if err != nil {
		t.Fatalf("创建资源失败: %v", err)
	}

	// 1. 创建即产生首版（action=create）。
	items, total, err := h.rev.List(ctx, domain.RevisionContentResource, res.ID.String(), 1, 20)
	if err != nil {
		t.Fatalf("查询历史失败: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].RevisionNo != 1 || items[0].Action != domain.RevisionActionCreate {
		t.Fatalf("期望创建后 1 条 create 版本，实际 total=%d items=%+v", total, items)
	}

	// 2. 更新名称 → 追加 update 版本，change_summary 含 name。
	newName := "更新后名称"
	if _, err := h.res.Update(ctx, res.ID.String(), &resource.UpdateInput{
		Name:      &newName,
		UpdatedBy: h.actorID,
		ActorName: "it-admin",
	}); err != nil {
		t.Fatalf("更新资源失败: %v", err)
	}
	items, total, err = h.rev.List(ctx, domain.RevisionContentResource, res.ID.String(), 1, 20)
	if err != nil {
		t.Fatalf("查询历史失败: %v", err)
	}
	if total != 2 || items[0].RevisionNo != 2 || items[0].Action != domain.RevisionActionUpdate {
		t.Fatalf("期望 2 条历史且最新为 update，实际 total=%d items=%+v", total, items)
	}
	if !contains(items[0].ChangeSummary, "name") {
		t.Fatalf("change_summary 应包含 name，实际 %v", items[0].ChangeSummary)
	}

	// 3. 差异查看：字段级 before/after。
	diff, err := h.rev.Diff(ctx, domain.RevisionContentResource, res.ID.String(), 2)
	if err != nil {
		t.Fatalf("查看差异失败: %v", err)
	}
	if diff.Previous == nil {
		t.Fatal("版本 2 应存在上一版")
	}
	var nameChange *revision.FieldChange
	for i := range diff.Changes {
		if diff.Changes[i].Field == "name" {
			nameChange = &diff.Changes[i]
		}
	}
	if nameChange == nil {
		t.Fatalf("差异应包含 name，实际 %+v", diff.Changes)
	}
	if !strings.Contains(string(nameChange.Before), "原始名称") || !strings.Contains(string(nameChange.After), "更新后名称") {
		t.Fatalf("name 差异前后值不符: before=%s after=%s", nameChange.Before, nameChange.After)
	}

	// 4. 回滚到版本 1 → 追加 rollback 版本，内容恢复。
	rec, err := h.rev.Rollback(ctx, domain.RevisionContentResource, res.ID.String(), 1, h.actorID, "it-admin", "回滚验证")
	if err != nil {
		t.Fatalf("回滚失败: %v", err)
	}
	if rec.RevisionNo != 3 || rec.Action != domain.RevisionActionRollback {
		t.Fatalf("回滚应追加第 3 版且 action=rollback，实际 no=%d action=%s", rec.RevisionNo, rec.Action)
	}

	restored, err := h.res.GetByID(ctx, res.ID.String())
	if err != nil {
		t.Fatalf("读取资源失败: %v", err)
	}
	if restored.Name != "原始名称" {
		t.Fatalf("回滚后 name 应为原始名称，实际 %q", restored.Name)
	}

	// 历史只增不减：回滚不删除既有版本。
	_, total, err = h.rev.List(ctx, domain.RevisionContentResource, res.ID.String(), 1, 20)
	if err != nil {
		t.Fatalf("查询历史失败: %v", err)
	}
	if total != 3 {
		t.Fatalf("回滚后应保留 3 条历史，实际 %d", total)
	}
}

// TestToolAndWikiRollbackIntegration 覆盖工具与百科的回滚还原。
func TestToolAndWikiRollbackIntegration(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	tool, err := h.content.CreateTool(ctx, &content.ToolInput{
		Slug:      "it-rev-tool",
		Name:      "工具原名",
		Summary:   "工具摘要",
		Category:  domain.ToolCategoryQuery,
		Status:    domain.StatusPublished,
		ActorID:   h.actorID,
		ActorName: "it-admin",
	})
	if err != nil {
		t.Fatalf("创建工具失败: %v", err)
	}
	if _, err := h.content.UpdateTool(ctx, tool.ID.String(), &content.ToolInput{
		Name:      "工具新名",
		Summary:   "工具摘要",
		Category:  domain.ToolCategoryQuery,
		Status:    domain.StatusPublished,
		ActorID:   h.actorID,
		ActorName: "it-admin",
	}); err != nil {
		t.Fatalf("更新工具失败: %v", err)
	}
	if _, err := h.rev.Rollback(ctx, domain.RevisionContentTool, tool.ID.String(), 1, h.actorID, "it-admin", ""); err != nil {
		t.Fatalf("工具回滚失败: %v", err)
	}
	restoredTool, err := h.content.GetToolByID(ctx, tool.ID.String())
	if err != nil {
		t.Fatalf("读取工具失败: %v", err)
	}
	if restoredTool.Name != "工具原名" {
		t.Fatalf("工具回滚后 name 应为原名，实际 %q", restoredTool.Name)
	}

	entry, err := h.content.CreateWiki(ctx, &content.WikiInput{
		Slug:      "it-rev-wiki",
		Title:     "百科原名",
		Content:   "初始内容",
		Status:    domain.StatusPublished,
		ActorName: "it-admin",
	}, h.actorID)
	if err != nil {
		t.Fatalf("创建百科失败: %v", err)
	}
	if _, err := h.content.UpdateWiki(ctx, entry.ID.String(), &content.WikiInput{
		Title:     "百科新名",
		Content:   "修改后的内容",
		Status:    domain.StatusPublished,
		ActorName: "it-admin",
	}, h.actorID); err != nil {
		t.Fatalf("更新百科失败: %v", err)
	}
	if _, err := h.rev.Rollback(ctx, domain.RevisionContentWiki, entry.ID.String(), 1, h.actorID, "it-admin", ""); err != nil {
		t.Fatalf("百科回滚失败: %v", err)
	}
	restoredEntry, err := h.content.GetWikiByID(ctx, entry.ID.String())
	if err != nil {
		t.Fatalf("读取百科失败: %v", err)
	}
	if restoredEntry.Title != "百科原名" || restoredEntry.Content != "初始内容" {
		t.Fatalf("百科回滚后内容不符: title=%q content=%q", restoredEntry.Title, restoredEntry.Content)
	}
}

// TestConcurrentUpdatesDistinctRevisionNumbersIntegration 验证并发更新不会产生重复版本号。
//
// 版本号由会话级咨询锁串行化分配；若失效，并发写入会撞上唯一索引而报错或产生重号。
func TestConcurrentUpdatesDistinctRevisionNumbersIntegration(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	res, err := h.res.Create(ctx, &resource.CreateInput{
		Slug:         "it-rev-concurrent",
		Name:         "并发原值",
		Summary:      "并发摘要",
		ResourceType: domain.ResourceTypeMod,
		SourceURL:    "https://example.com/concurrent",
		Status:       domain.StatusPublished,
		CreatedBy:    h.actorID,
		ActorName:    "it-admin",
	})
	if err != nil {
		t.Fatalf("创建资源失败: %v", err)
	}

	const n = 6
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := "并发值-" + string(rune('A'+i))
			_, e := h.res.Update(ctx, res.ID.String(), &resource.UpdateInput{
				Name:      &name,
				UpdatedBy: h.actorID,
				ActorName: "it-admin",
			})
			errs[i] = e
		}(i)
	}
	wg.Wait()

	for i, e := range errs {
		if e != nil {
			t.Fatalf("第 %d 次并发更新失败: %v", i, e)
		}
	}

	items, total, err := h.rev.List(ctx, domain.RevisionContentResource, res.ID.String(), 1, 50)
	if err != nil {
		t.Fatalf("查询历史失败: %v", err)
	}
	if total != int64(n+1) {
		t.Fatalf("期望 %d 条历史，实际 %d", n+1, total)
	}
	seen := make(map[int]struct{}, len(items))
	for _, it := range items {
		if _, dup := seen[it.RevisionNo]; dup {
			t.Fatalf("出现重复版本号: %d", it.RevisionNo)
		}
		seen[it.RevisionNo] = struct{}{}
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
