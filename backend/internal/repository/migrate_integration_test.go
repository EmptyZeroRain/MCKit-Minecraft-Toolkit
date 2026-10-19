package db_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"gorm.io/gorm"

	db "mckit/backend/internal/repository"
)

// migrationsDir 相对本包测试工作目录定位迁移目录。
const migrationsDir = "../../migrations"

// migrateSchema 本包集成测试专用 schema，避免与其他包（cmd/api 会 DROP SCHEMA public）互相干扰。
const migrateSchema = "migrate_it"

// TestMigrationsHaveDownFiles 保证每个正向迁移都有对应的反向文件。
//
// 迁移规程要求（gj.md §16.4）：缺失 down 文件会让回退/恢复演练无法进行，
// 因此把它固化为自动化检查，而不是靠人工记忆。
func TestMigrationsHaveDownFiles(t *testing.T) {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("读取迁移目录失败: %v", err)
	}
	down := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".down.sql") {
			down[strings.TrimSuffix(name, ".down.sql")] = true
		}
	}
	upCount := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") || strings.HasSuffix(name, ".down.sql") {
			continue
		}
		upCount++
		base := strings.TrimSuffix(name, ".sql")
		if !down[base] {
			t.Errorf("迁移 %s 缺少反向文件 %s.down.sql", name, base)
		}
	}
	if upCount == 0 {
		t.Fatal("未发现任何正向迁移文件")
	}
}

// TestMigrateRollbackRoundTrip 验证「正向 → 全部回退 → 再正向」可重复执行。
//
// 这是备份恢复演练的自动化基线：如果回退不可用，生产回滚就只能靠重建数据库。
func TestMigrateRollbackRoundTrip(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("未设置 TEST_DATABASE_URL，跳过数据库集成测试")
	}
	ctx := context.Background()

	admin, err := db.Open(url)
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	defer func() { _ = admin.Exec("DROP SCHEMA IF EXISTS " + migrateSchema + " CASCADE") }()
	if err := admin.Exec("DROP SCHEMA IF EXISTS " + migrateSchema + " CASCADE").Error; err != nil {
		t.Fatalf("清理 schema 失败: %v", err)
	}
	if err := admin.Exec("CREATE SCHEMA " + migrateSchema).Error; err != nil {
		t.Fatalf("创建 schema 失败: %v", err)
	}

	gormDB, err := db.Open(withSearchPath(url, migrateSchema))
	if err != nil {
		t.Fatalf("连接 schema 失败: %v", err)
	}

	// 1) 正向：应用全部迁移。
	applied, err := db.Migrate(ctx, gormDB, migrationsDir)
	if err != nil {
		t.Fatalf("正向迁移失败: %v", err)
	}
	if len(applied) == 0 {
		t.Fatal("正向迁移未执行任何文件")
	}
	if !tableExists(t, gormDB, "accounts") {
		t.Fatal("迁移后应存在 accounts 表")
	}

	// 2) 全部回退：清空 schema_migrations 与所有业务表。
	if _, err := db.Rollback(ctx, gormDB, migrationsDir, len(applied)); err != nil {
		t.Fatalf("回退全部迁移失败: %v", err)
	}
	var remaining int64
	if err := gormDB.Table("schema_migrations").Count(&remaining).Error; err != nil {
		t.Fatalf("统计 schema_migrations 失败: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("回退后 schema_migrations 应为空，实际 %d 条", remaining)
	}
	if tableExists(t, gormDB, "accounts") {
		t.Fatal("回退后不应存在 accounts 表")
	}

	// 3) 再正向：验证迁移可重复执行（恢复演练）。
	if _, err := db.Migrate(ctx, gormDB, migrationsDir); err != nil {
		t.Fatalf("回退后重新迁移失败: %v", err)
	}
	if !tableExists(t, gormDB, "accounts") {
		t.Fatal("重新迁移后应恢复 accounts 表")
	}
}

// withSearchPath 在 DSN 上追加 search_path 参数。
func withSearchPath(dsn, schema string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + schema
}

// tableExists 判断当前 search_path 下是否存在指定表。
func tableExists(t *testing.T, gormDB *gorm.DB, table string) bool {
	t.Helper()
	var n int64
	err := gormDB.Raw(
		"SELECT count(*) FROM information_schema.tables WHERE table_name = ? AND table_schema = current_schema()",
		table,
	).Scan(&n).Error
	if err != nil {
		t.Fatalf("查询表是否存在失败: %v", err)
	}
	return n > 0
}
