// Package db 提供 PostgreSQL 连接（GORM）与轻量迁移执行器。
package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open 建立 GORM 连接并验证连通性。
func Open(databaseURL string) (*gorm.DB, error) {
	gormDB, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("创建数据库连接失败: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("获取底层连接失败: %w", err)
	}
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	// 启动时验证连通性，避免带病启动。
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("数据库连接失败: %w", err)
	}
	return gormDB, nil
}

// 迁移文件命名约定：
//   - 正向：<序号>_<名称>.sql
//   - 反向：<序号>_<名称>.down.sql（可缺省，表示该迁移不支持自动回滚）
//
// 反向文件不参与正向迁移扫描；回退时按版本降序执行并清除 schema_migrations 记录。

// migration 一个版本的正 / 反向迁移文件。
type migration struct {
	Version int64
	Name    string
	// Up 正向文件路径。
	Up string
	// Down 反向文件路径；为空表示没有反向迁移。
	Down string
}

// Migrate 顺序执行 migrations 目录中所有未应用的正向迁移。
// 已应用的迁移通过 schema_migrations 表记录，重复执行安全。
func Migrate(ctx context.Context, gormDB *gorm.DB, dir string) ([]string, error) {
	if err := ensureMigrationTable(gormDB); err != nil {
		return nil, err
	}
	all, err := collectMigrations(dir)
	if err != nil {
		return nil, err
	}
	applied, err := loadApplied(gormDB)
	if err != nil {
		return nil, err
	}

	var executed []string
	for _, m := range all {
		if applied[m.Version] {
			continue
		}
		if err := applyUp(gormDB, m); err != nil {
			return nil, err
		}
		executed = append(executed, m.Name)
	}
	return executed, nil
}

// MigrateTo 把数据库迁移到指定版本（可前进或回退）。
// target 必须对应迁移目录中存在的版本；高于当前版本则前进，低于则回退。
func MigrateTo(ctx context.Context, gormDB *gorm.DB, dir string, target int64) ([]string, error) {
	if err := ensureMigrationTable(gormDB); err != nil {
		return nil, err
	}
	all, err := collectMigrations(dir)
	if err != nil {
		return nil, err
	}
	known := false
	for _, m := range all {
		if m.Version == target {
			known = true
			break
		}
	}
	if !known {
		return nil, fmt.Errorf("目标版本 %d 不存在于迁移目录", target)
	}
	applied, err := loadApplied(gormDB)
	if err != nil {
		return nil, err
	}

	current := int64(0)
	for v := range applied {
		if v > current {
			current = v
		}
	}

	switch {
	case target > current:
		var executed []string
		for _, m := range all {
			if m.Version > target || applied[m.Version] {
				continue
			}
			if err := applyUp(gormDB, m); err != nil {
				return nil, err
			}
			executed = append(executed, m.Name)
		}
		return executed, nil
	case target < current:
		return rollbackTo(gormDB, all, applied, target)
	default:
		return nil, nil
	}
}

// Rollback 回退最近 steps 个已应用的迁移（按版本降序）。
func Rollback(ctx context.Context, gormDB *gorm.DB, dir string, steps int) ([]string, error) {
	if steps <= 0 {
		return nil, fmt.Errorf("回退步数必须为正数")
	}
	if err := ensureMigrationTable(gormDB); err != nil {
		return nil, err
	}
	all, err := collectMigrations(dir)
	if err != nil {
		return nil, err
	}
	applied, err := loadApplied(gormDB)
	if err != nil {
		return nil, err
	}

	versions := make([]int64, 0, len(applied))
	for v := range applied {
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] > versions[j] })
	if steps > len(versions) {
		return nil, fmt.Errorf("已应用迁移只有 %d 个，无法回退 %d 步", len(versions), steps)
	}
	target := int64(0)
	if steps < len(versions) {
		target = versions[steps]
	}
	return rollbackTo(gormDB, all, applied, target)
}

// rollbackTo 回退所有版本 > target 的已应用迁移（降序，逐版本单事务）。
func rollbackTo(gormDB *gorm.DB, all []migration, applied map[int64]bool, target int64) ([]string, error) {
	byVersion := make(map[int64]migration, len(all))
	for _, m := range all {
		byVersion[m.Version] = m
	}

	versions := make([]int64, 0, len(applied))
	for v := range applied {
		if v > target {
			versions = append(versions, v)
		}
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] > versions[j] })

	var executed []string
	for _, v := range versions {
		m, ok := byVersion[v]
		if !ok {
			return nil, fmt.Errorf("版本 %d 已应用，但迁移目录中找不到对应文件，无法回退", v)
		}
		if m.Down == "" {
			return nil, fmt.Errorf(
				"版本 %d（%s）缺少反向迁移文件 %d_%s.down.sql，无法自动回退",
				v, m.Name, v, m.Name)
		}
		content, err := os.ReadFile(m.Down)
		if err != nil {
			return nil, fmt.Errorf("[%s] 读取反向迁移文件失败: %w", m.Name, err)
		}

		tx := gormDB.Begin()
		if tx.Error != nil {
			return nil, fmt.Errorf("[%s] 开启事务失败: %w", m.Name, tx.Error)
		}
		if err := tx.Exec(string(content)).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("[%s] 反向迁移执行失败: %w", m.Name, err)
		}
		if err := tx.Exec("DELETE FROM schema_migrations WHERE version = ?", v).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("[%s] 清除迁移记录失败: %w", m.Name, err)
		}
		if err := tx.Commit().Error; err != nil {
			return nil, fmt.Errorf("[%s] 提交反向迁移失败: %w", m.Name, err)
		}
		executed = append(executed, m.Name+".down")
	}
	return executed, nil
}

// applyUp 在单事务中执行一个正向迁移并记录版本（失败整体回滚）。
func applyUp(gormDB *gorm.DB, m migration) error {
	content, err := os.ReadFile(m.Up)
	if err != nil {
		return fmt.Errorf("[%s] 读取迁移文件失败: %w", m.Name, err)
	}

	tx := gormDB.Begin()
	if tx.Error != nil {
		return fmt.Errorf("[%s] 开启事务失败: %w", m.Name, tx.Error)
	}
	if err := tx.Exec(string(content)).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("[%s] 迁移执行失败: %w", m.Name, err)
	}
	if err := tx.Exec(
		"INSERT INTO schema_migrations (version, name) VALUES (?, ?)",
		m.Version, m.Name).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("[%s] 记录迁移状态失败: %w", m.Name, err)
	}
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("[%s] 提交事务失败: %w", m.Name, err)
	}
	return nil
}

// collectMigrations 扫描迁移目录，按版本升序返回正向 / 反向文件对。
func collectMigrations(dir string) ([]migration, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return nil, fmt.Errorf("读取迁移目录失败: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("迁移目录 %s 中没有 .sql 文件", dir)
	}
	sort.Strings(files)

	index := make(map[int64]*migration)
	for _, path := range files {
		base := filepath.Base(path)
		version, name, isDown, err := parseMigrationName(base)
		if err != nil {
			return nil, err
		}
		m, ok := index[version]
		if !ok {
			m = &migration{Version: version, Name: name}
			index[version] = m
		}
		if m.Name != name {
			return nil, fmt.Errorf("迁移版本 %d 出现不同名称: %s / %s", version, m.Name, name)
		}
		if isDown {
			if m.Down != "" {
				return nil, fmt.Errorf("迁移版本 %d 存在重复的反向文件: %s", version, base)
			}
			m.Down = path
			continue
		}
		if m.Up != "" {
			return nil, fmt.Errorf("迁移版本 %d 存在重复的正向文件: %s", version, base)
		}
		m.Up = path
	}

	out := make([]migration, 0, len(index))
	for _, m := range index {
		if m.Up == "" {
			return nil, fmt.Errorf("迁移版本 %d（%s）缺少正向文件", m.Version, m.Name)
		}
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

func ensureMigrationTable(gormDB *gorm.DB) error {
	return gormDB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version BIGINT PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`).Error
}

func loadApplied(gormDB *gorm.DB) (map[int64]bool, error) {
	type row struct {
		Version int64
	}
	var rows []row
	if err := gormDB.Table("schema_migrations").Find(&rows).Error; err != nil {
		return nil, err
	}
	set := make(map[int64]bool, len(rows))
	for _, r := range rows {
		set[r.Version] = true
	}
	return set, nil
}

// parseMigrationName 解析形如 "0001_create_admins.sql" 的迁移文件名，
// 并识别 ".down.sql" 后缀（反向迁移）。
func parseMigrationName(base string) (version int64, name string, isDown bool, err error) {
	idx := strings.IndexByte(base, '_')
	if idx <= 0 {
		return 0, "", false, fmt.Errorf("迁移文件名 %s 格式非法，应为 <序号>_<名称>.sql", base)
	}
	if _, err := fmt.Sscanf(base[:idx], "%d", &version); err != nil {
		return 0, "", false, fmt.Errorf("迁移文件名 %s 序号非法: %w", base, err)
	}
	rest := strings.TrimSuffix(base[idx+1:], ".sql")
	if strings.HasSuffix(rest, ".down") {
		return version, strings.TrimSuffix(rest, ".down"), true, nil
	}
	return version, rest, false, nil
}
