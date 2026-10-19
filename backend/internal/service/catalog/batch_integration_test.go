package catalog_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	db "mckit/backend/internal/repository"
	"mckit/backend/internal/service/catalog"
)

// catalogSchema 本包集成测试专用 schema（与其他包的 public / *_it 隔离）。
const catalogSchema = "catalog_it"

func withSearchPath(dsn, schema string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + schema
}

func setupCatalogDB(t *testing.T) *gorm.DB {
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
	if err := adminDB.Exec("DROP SCHEMA IF EXISTS " + catalogSchema + " CASCADE").Error; err != nil {
		t.Fatalf("清理测试 schema 失败: %v", err)
	}
	if err := adminDB.Exec("CREATE SCHEMA " + catalogSchema).Error; err != nil {
		t.Fatalf("创建测试 schema 失败: %v", err)
	}
	if sqlDB, err := adminDB.DB(); err == nil {
		_ = sqlDB.Close()
	}

	gdb, err := db.Open(withSearchPath(url, catalogSchema))
	if err != nil {
		t.Fatalf("连接测试 schema 失败: %v", err)
	}
	if _, err := db.Migrate(ctx, gdb, "../../../migrations"); err != nil {
		t.Fatalf("执行迁移失败: %v", err)
	}
	return gdb
}

// seedMods 直接插入 n 条 pending 状态的 Mod 记录（内容由外部导入，服务层无 Create）。
func seedMods(t *testing.T, gdb *gorm.DB, n int) []domain.Mod {
	t.Helper()
	out := make([]domain.Mod, 0, n)
	for i := 0; i < n; i++ {
		m := domain.Mod{
			Slug:         "batch-mod-" + string(rune('a'+i)),
			NameZh:       "批量测试 Mod " + string(rune('A'+i)),
			GameEdition:  "java",
			ReviewStatus: domain.ModReviewPending,
		}
		if err := gdb.Create(&m).Error; err != nil {
			t.Fatalf("创建 Mod 失败: %v", err)
		}
		out = append(out, m)
	}
	return out
}

func TestBatchReviewModsPartial(t *testing.T) {
	gdb := setupCatalogDB(t)
	ctx := context.Background()
	svc := catalog.NewService(gdb)
	mods := seedMods(t, gdb, 3)

	ids := []string{mods[0].ID.String(), mods[1].ID.String(), "00000000-0000-0000-0000-000000000000"}
	result, err := svc.BatchReviewMods(ctx, ids, domain.ModReviewPublished, "reviewer", "批量通过")
	if err != nil {
		t.Fatalf("批量审核失败: %v", err)
	}
	if result.Requested != 3 || result.Succeeded != 2 || result.Failed != 1 {
		t.Fatalf("汇总异常: %+v", result)
	}

	// 不存在的记录应带可展示原因，且不泄露内部错误。
	for _, item := range result.Items {
		if item.ID == ids[2] {
			if item.OK || item.Error == "" {
				t.Fatalf("不存在的记录应失败并带原因: %+v", item)
			}
		}
	}

	// 审核结果确实落库。
	var published int64
	if err := gdb.Model(&domain.Mod{}).
		Where("review_status = ? AND reviewer = ?", domain.ModReviewPublished, "reviewer").
		Count(&published).Error; err != nil {
		t.Fatal(err)
	}
	if published != 2 {
		t.Fatalf("已发布数量 = %d, want 2", published)
	}
}

func TestBatchReviewModsValidation(t *testing.T) {
	gdb := setupCatalogDB(t)
	ctx := context.Background()
	svc := catalog.NewService(gdb)

	if _, err := svc.BatchReviewMods(ctx, nil, domain.ModReviewPublished, "r", ""); !errors.Is(err, catalog.ErrNoIDs) {
		t.Fatalf("空列表应返回 ErrNoIDs，实际 %v", err)
	}
	tooMany := make([]string, domain.MaxBatchReview+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("id-%d", i)
	}
	if _, err := svc.BatchReviewMods(ctx, tooMany, domain.ModReviewPublished, "r", ""); !errors.Is(err, catalog.ErrTooManyIDs) {
		t.Fatalf("超上限应返回 ErrTooManyIDs，实际 %v", err)
	}
}

// TestBatchReviewRedstoneDesigns 红石设计的批量审核与单条语义一致。
func TestBatchReviewRedstoneDesigns(t *testing.T) {
	gdb := setupCatalogDB(t)
	ctx := context.Background()
	svc := catalog.NewService(gdb)

	design := domain.RedstoneDesign{
		Slug:         "batch-design",
		NameZh:       "批量红石设计",
		Purpose:      "other",
		Difficulty:   "medium",
		ReviewStatus: domain.ModReviewPending,
	}
	if err := gdb.Create(&design).Error; err != nil {
		t.Fatalf("创建设计失败: %v", err)
	}

	result, err := svc.BatchReviewRedstoneDesigns(ctx, []string{design.ID.String()},
		domain.ModReviewPublished, "reviewer", "")
	if err != nil {
		t.Fatalf("批量审核失败: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("应成功 1 条: %+v", result)
	}
	if result.Items[0].Label != "批量红石设计" {
		t.Fatalf("应回填可读名称，实际 %q", result.Items[0].Label)
	}
}
