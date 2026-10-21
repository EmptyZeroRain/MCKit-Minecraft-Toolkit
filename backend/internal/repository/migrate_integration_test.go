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

// TestMigrateRepairsLegacyApplied44 覆盖线上曾出现的状态：
// schema_migrations 已记录 version=44（旧名称），但新 0044 的曲线表并未建立。
// 0045 应补建 growth_level_config / growth_level_titles，并移除旧 growth_levels。
func TestMigrateRepairsLegacyApplied44(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("未设置 TEST_DATABASE_URL，跳过数据库集成测试")
	}
	const schema = "migrate_broken_44"
	ctx := context.Background()

	admin, err := db.Open(url)
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	defer func() { _ = admin.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE") }()
	if err := admin.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE").Error; err != nil {
		t.Fatalf("清理 schema 失败: %v", err)
	}
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("创建 schema 失败: %v", err)
	}

	gormDB, err := db.Open(withSearchPath(url, schema))
	if err != nil {
		t.Fatalf("连接 schema 失败: %v", err)
	}

	// 先迁移到 0043，模拟旧版数据库仍有 growth_levels。
	if _, err := db.MigrateTo(ctx, gormDB, migrationsDir, 43); err != nil {
		t.Fatalf("迁移到 0043 失败: %v", err)
	}
	if !tableExists(t, gormDB, "growth_levels") {
		t.Fatal("0043 后应仍存在旧 growth_levels 表")
	}
	if tableExists(t, gormDB, "growth_level_config") {
		t.Fatal("0043 后不应存在 growth_level_config 表")
	}

	// 模拟旧 0044 已被记录为已应用，但实际没有建表。
	if err := gormDB.Exec(
		"INSERT INTO schema_migrations (version, name) VALUES (44, 'growth_level_titles')",
	).Error; err != nil {
		t.Fatalf("写入旧 0044 迁移记录失败: %v", err)
	}

	applied, err := db.Migrate(ctx, gormDB, migrationsDir)
	if err != nil {
		t.Fatalf("执行修复迁移失败: %v", err)
	}
	// 0045 / 0046 / 0047 / 0048 / 0049 / 0050 必须按顺序执行；后续迁移由其他功能追加，不在此断言。
	wantApplied := []string{
		"growth_level_reconcile",
		"growth_migration_metadata",
		"growth_unlimited_levels",
		"checkin_xp_and_makeup_cards",
		"gift_stock",
		"growth_level_title_constraints",
	}
	if len(applied) < len(wantApplied) {
		t.Fatalf("应至少执行 %d 个修复迁移，实际: %v", len(wantApplied), applied)
	}
	for i, want := range wantApplied {
		if applied[i] != want {
			t.Fatalf("第 %d 个修复迁移应为 %s，实际: %v", i+1, want, applied)
		}
	}
	if !tableExists(t, gormDB, "gift_checkin_xp") || !tableExists(t, gormDB, "gift_checkin_streaks") ||
		!tableExists(t, gormDB, "gift_makeup_cards") || !tableExists(t, gormDB, "gift_checkin_milestones") {
		t.Fatal("0048 后应存在签到经验、连续签到、补签卡与里程碑表")
	}
	var stockColumns int64
	if err := gormDB.Raw(`SELECT count(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
		"gift_definitions", "stock").Scan(&stockColumns).Error; err != nil {
		t.Fatalf("查询礼物库存字段失败: %v", err)
	}
	if stockColumns != 1 {
		t.Fatal("0049 后 gift_definitions 应存在 stock 字段")
	}
	var legacyLevelChecks int64
	if err := gormDB.Raw(`SELECT count(*) FROM pg_constraint
		WHERE conrelid = 'growth_level_titles'::regclass
		  AND contype = 'c'
		  AND pg_get_constraintdef(oid) LIKE '%9999%'`).Scan(&legacyLevelChecks).Error; err != nil {
		t.Fatalf("查询称号范围约束失败: %v", err)
	}
	if legacyLevelChecks != 0 {
		t.Fatalf("0050 后不应残留 9999 称号上限约束，实际 %d 条", legacyLevelChecks)
	}
	if !tableExists(t, gormDB, "growth_level_config") {
		t.Fatal("修复后应存在 growth_level_config 表")
	}
	if !tableExists(t, gormDB, "growth_level_titles") {
		t.Fatal("修复后应存在 growth_level_titles 表")
	}
	if tableExists(t, gormDB, "growth_levels") {
		t.Fatal("修复后旧 growth_levels 应被删除")
	}

	var configCount int64
	if err := gormDB.Table("growth_level_config").Where("id = 1").Count(&configCount).Error; err != nil {
		t.Fatalf("查询等级配置失败: %v", err)
	}
	if configCount != 1 {
		t.Fatalf("等级配置应有一条默认记录，实际 %d", configCount)
	}
	var maxLevel int
	if err := gormDB.Table("growth_level_config").Where("id = 1").Pluck("max_level", &maxLevel).Error; err != nil {
		t.Fatalf("查询等级上限失败: %v", err)
	}
	if maxLevel != 0 {
		t.Fatalf("等级上限应为 0（不限），实际 %d", maxLevel)
	}
	var titleCount int64
	if err := gormDB.Table("growth_level_titles").Count(&titleCount).Error; err != nil {
		t.Fatalf("查询称号规则失败: %v", err)
	}
	if titleCount == 0 {
		t.Fatal("修复后应从旧 growth_levels 回填称号规则")
	}

	var migrationName string
	if err := gormDB.Table("schema_migrations").
		Where("version = 44").
		Pluck("name", &migrationName).Error; err != nil {
		t.Fatalf("查询 0044 迁移记录失败: %v", err)
	}
	if migrationName != "growth_infinite_levels" {
		t.Fatalf("0044 迁移名称应修正为 growth_infinite_levels，实际 %q", migrationName)
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
