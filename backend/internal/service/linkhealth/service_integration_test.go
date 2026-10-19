package linkhealth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/linkcheck"
	db "mckit/backend/internal/repository"
	"mckit/backend/internal/repository/types"
)

// linkhealthSchema 本包集成测试专用 schema。
//
// 背景：本包与 cmd/api 集成测试共用同一个测试库（TEST_DATABASE_URL），
// 而 go test 默认并行跑包，cmd/api 的 integrationSetup 会执行
// `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`，会把本包正在使用的
// 表与数据一并删掉，导致本包偶发失败（曾出现 TestRunCheckAndConfirmIntegration 失败）。
// 因此本包把对象全部建在独立 schema 中，与 public 隔离。
const linkhealthSchema = "linkhealth_it"

// withSearchPath 在 DSN 上追加 search_path 运行时参数。
// pgx（GORM postgres 驱动的默认实现）会把连接串中的未知参数作为运行时参数下发，
// 因此连接池里每条新连接都带上该 search_path，无需逐连接执行 SET。
func withSearchPath(dsn, schema string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + schema
}

// 链接健康集成测试：验证「批量复查 → 落库 → 更新资源链接状态 → 人工确认 → 复查跳过已确认项」全链路。
//
// 需要独立测试库（与 cmd/api 集成测试同口径）：
//
//	TEST_DATABASE_URL='postgres://user:pass@127.0.0.1:5432/mckit_linktest?sslmode=disable' go test ./internal/service/linkhealth
func TestRunCheckAndConfirmIntegration(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("未设置 TEST_DATABASE_URL，跳过数据库集成测试")
	}
	ctx := context.Background()

	// 1) 先在默认 schema 下重建本包专用 schema，避免与 cmd/api 的 public 相互干扰。
	adminDB, err := db.Open(url)
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	if err := adminDB.Exec("DROP SCHEMA IF EXISTS " + linkhealthSchema + " CASCADE").Error; err != nil {
		t.Fatalf("清理测试 schema 失败: %v", err)
	}
	if err := adminDB.Exec("CREATE SCHEMA " + linkhealthSchema).Error; err != nil {
		t.Fatalf("创建测试 schema 失败: %v", err)
	}
	if sqlDB, err := adminDB.DB(); err == nil {
		_ = sqlDB.Close()
	}

	// 2) 用带 search_path 的连接执行迁移与断言。
	gdb, err := db.Open(withSearchPath(url, linkhealthSchema))
	if err != nil {
		t.Fatalf("连接测试 schema 失败: %v", err)
	}
	if _, err := db.Migrate(ctx, gdb, "../../../migrations"); err != nil {
		t.Fatalf("执行迁移失败: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	slug := "it-linkhealth-" + time.Now().Format("150405.000000")
	res := domain.Resource{
		Slug:              slug,
		Name:              "集成测试资源",
		Summary:           "用于链接健康集成测试",
		ResourceType:      "mod",
		SourceURL:         srv.URL + "/download",
		GameEdition:       domain.GameEditionJava,
		SupportedVersions: types.JSONSlice{},
		LoaderTypes:       types.JSONSlice{},
		Dependencies:      types.JSONB{},
		Tags:              types.JSONSlice{},
		CompatibilityNote: "",
		Status:            domain.StatusPublished,
		LinkStatus:        domain.LinkStatusUnchecked,
	}
	if err := gdb.Create(&res).Error; err != nil {
		t.Fatalf("创建测试资源失败: %v", err)
	}
	t.Cleanup(func() {
		gdb.Where("resource_id = ?", res.ID).Delete(&domain.LinkCheck{})
		gdb.Unscoped().Where("id = ?", res.ID).Delete(&domain.Resource{})
	})

	checker := linkcheck.NewChecker(3*time.Second, 3)
	checker.AllowPrivateIP = true // 仅测试：httptest 监听回环地址
	svc := NewService(gdb, checker, 2)

	summary, err := svc.RunCheck(ctx, RunOptions{ResourceID: res.ID.String()})
	if err != nil {
		t.Fatalf("批量复查失败: %v", err)
	}
	if summary.Checked != 1 || summary.OK != 1 {
		t.Fatalf("期望检查 1 条且正常 1 条，实际 %+v", summary)
	}
	if summary.Concurrency != 2 {
		t.Fatalf("并发上限应为 2，实际 %d", summary.Concurrency)
	}

	// 资源链接状态应被同步更新。
	var updated domain.Resource
	if err := gdb.First(&updated, "id = ?", res.ID).Error; err != nil {
		t.Fatalf("读取资源失败: %v", err)
	}
	if updated.LinkStatus != domain.LinkStatusOK || updated.LastCheckedAt == nil {
		t.Fatalf("资源链接状态未更新: %+v", updated.LinkStatus)
	}

	list, err := svc.List(ctx, ListFilter{FailedOnly: false, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("查询复核列表失败: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("期望 1 条复核记录，实际 %d", len(list.Items))
	}
	item := list.Items[0]
	if item.ResourceSlug != slug || item.ReviewState != domain.ReviewPending {
		t.Fatalf("复核记录内容不符: %+v", item)
	}

	confirmed, err := svc.Confirm(ctx, item.ID, "confirm_ok", "it-admin", "本地服务可达")
	if err != nil {
		t.Fatalf("人工确认失败: %v", err)
	}
	if confirmed.ReviewState != domain.ReviewConfirmedOK || confirmed.ReviewedBy != "it-admin" {
		t.Fatalf("确认结果不符: %+v", confirmed)
	}

	// 已确认可用的资源，OnlyPending 复查应跳过。
	again, err := svc.RunCheck(ctx, RunOptions{ResourceID: res.ID.String(), OnlyPending: true})
	if err != nil {
		t.Fatalf("二次复查失败: %v", err)
	}
	if again.Checked != 0 {
		t.Fatalf("已确认可用项应被跳过，实际检查 %d 条", again.Checked)
	}
}
