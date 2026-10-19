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

// Migrate 顺序执行 migrations 目录中的 .sql 文件。
// 已应用的迁移通过 schema_migrations 表记录，重复执行安全。
func Migrate(ctx context.Context, gormDB *gorm.DB, dir string) ([]string, error) {
	if err := ensureMigrationTable(gormDB); err != nil {
		return nil, err
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return nil, fmt.Errorf("读取迁移目录失败: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("迁移目录 %s 中没有 .sql 文件", dir)
	}
	sort.Strings(files)

	appliedSet, err := loadApplied(gormDB)
	if err != nil {
		return nil, err
	}

	var executed []string
	for _, path := range files {
		version, name, err := parseMigrationName(filepath.Base(path))
		if err != nil {
			return nil, err
		}
		if appliedSet[version] {
			continue
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("[%s] 读取迁移文件失败: %w", name, err)
		}

		// 整份迁移文件在单个事务中执行：失败则整体回滚。
		tx := gormDB.Begin()
		if tx.Error != nil {
			return nil, fmt.Errorf("[%s] 开启事务失败: %w", name, tx.Error)
		}
		if err := tx.Exec(string(content)).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("[%s] 迁移执行失败: %w", name, err)
		}
		if err := tx.Exec(
			"INSERT INTO schema_migrations (version, name) VALUES (?, ?)",
			version, name).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("[%s] 记录迁移状态失败: %w", name, err)
		}
		if err := tx.Commit().Error; err != nil {
			return nil, fmt.Errorf("[%s] 提交事务失败: %w", name, err)
		}
		executed = append(executed, name)
	}
	return executed, nil
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

// parseMigrationName 解析形如 "0001_create_admins.sql" 的迁移文件名。
func parseMigrationName(base string) (int64, string, error) {
	idx := strings.IndexByte(base, '_')
	if idx <= 0 {
		return 0, "", fmt.Errorf("迁移文件名 %s 格式非法，应为 <序号>_<名称>.sql", base)
	}
	var version int64
	if _, err := fmt.Sscanf(base[:idx], "%d", &version); err != nil {
		return 0, "", fmt.Errorf("迁移文件名 %s 序号非法: %w", base, err)
	}
	return version, strings.TrimSuffix(base[idx+1:], ".sql"), nil
}