package resource

import (
	"context"
	"os"
	"strings"
	"testing"

	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	dbpkg "mckit/backend/internal/repository"
)

const resourceSchema = "resource_it"

func setupResourceDB(t *testing.T) *gorm.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("未设置 TEST_DATABASE_URL，跳过数据库集成测试")
	}
	ctx := context.Background()

	adminDB, err := dbpkg.Open(url)
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	if err := adminDB.Exec("DROP SCHEMA IF EXISTS " + resourceSchema + " CASCADE").Error; err != nil {
		t.Fatalf("清理测试 schema 失败: %v", err)
	}
	if err := adminDB.Exec("CREATE SCHEMA " + resourceSchema).Error; err != nil {
		t.Fatalf("创建测试 schema 失败: %v", err)
	}
	if sqlDB, err := adminDB.DB(); err == nil {
		_ = sqlDB.Close()
	}

	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	gdb, err := dbpkg.Open(url + sep + "search_path=" + resourceSchema)
	if err != nil {
		t.Fatalf("连接测试 schema 失败: %v", err)
	}
	if _, err := dbpkg.Migrate(ctx, gdb, "../../../migrations"); err != nil {
		t.Fatalf("执行迁移失败: %v", err)
	}
	return gdb
}

func seedResource(t *testing.T, gdb *gorm.DB, slug, status string) {
	t.Helper()
	row := domain.Resource{
		Slug:         slug,
		Name:         "资源 " + slug,
		Summary:      "摘要",
		ResourceType: domain.ResourceTypeMod,
		SourceURL:    "https://example.com/" + slug,
		Status:       status,
	}
	if err := gdb.Create(&row).Error; err != nil {
		t.Fatalf("写入资源失败: %v", err)
	}
}

// TestStatusFilterActuallyApplies 回归：后台列表一直传 status，后端此前未接收。
func TestStatusFilterActuallyApplies(t *testing.T) {
	gdb := setupResourceDB(t)
	svc := NewService(gdb)

	seedResource(t, gdb, "jei", domain.StatusPublished)
	seedResource(t, gdb, "rei", domain.StatusPublished)
	seedResource(t, gdb, "draft-mod", domain.StatusDraft)

	items, total, err := svc.List(context.Background(), ListFilter{Status: domain.StatusDraft})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].Slug != "draft-mod" {
		t.Fatalf("状态筛选未生效: total=%d items=%d", total, len(items))
	}
}

// TestExportHonoursFilters 导出必须与列表同一套筛选条件。
func TestExportHonoursFilters(t *testing.T) {
	gdb := setupResourceDB(t)
	svc := NewService(gdb)

	for _, slug := range []string{"a", "b", "c"} {
		seedResource(t, gdb, slug, domain.StatusPublished)
	}
	seedResource(t, gdb, "d", domain.StatusDraft)

	items, truncated, err := svc.Export(context.Background(), ListFilter{Status: domain.StatusPublished})
	if err != nil {
		t.Fatalf("导出失败: %v", err)
	}
	if len(items) != 3 || truncated {
		t.Fatalf("导出条数异常: len=%d truncated=%v", len(items), truncated)
	}
	if items[0].Slug == "" || items[0].ID.String() == "" {
		t.Fatalf("导出缺字段: %+v", items[0])
	}

	// 关键词筛选同样要在导出中生效。
	items, _, err = svc.Export(context.Background(), ListFilter{Q: "资源 c"})
	if err != nil {
		t.Fatalf("导出失败: %v", err)
	}
	if len(items) != 1 || items[0].Slug != "c" {
		t.Fatalf("关键词筛选未生效: %+v", items)
	}
}
