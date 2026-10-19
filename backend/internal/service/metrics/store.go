package metrics

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mckit/backend/internal/domain"
)

// Store 聚合结果的持久化接口（便于用内存实现做单元测试）。
type Store interface {
	UpsertToolStats(ctx context.Context, rows []domain.ToolStatDaily) error
	UpsertResourceStats(ctx context.Context, rows []domain.ResourceStatDaily) error
	UpsertAPIStats(ctx context.Context, rows []domain.APIStatDaily) error
	UpsertSearchStats(ctx context.Context, rows []domain.SearchStatDaily) error
	// PruneBefore 删除 stat_date < cutoff 的所有日聚合行，返回删除行数。
	PruneBefore(ctx context.Context, cutoff time.Time) (int64, error)
}

// GormStore 基于 GORM 的实现。
type GormStore struct{ db *gorm.DB }

// NewGormStore 创建 GORM 持久化实现。
func NewGormStore(db *gorm.DB) *GormStore { return &GormStore{db: db} }

// UpsertToolStats 批量 upsert，冲突时按主键累加计数。
func (s *GormStore) UpsertToolStats(ctx context.Context, rows []domain.ToolStatDaily) error {
	if len(rows) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "stat_date"}, {Name: "tool_slug"}},
		DoUpdates: clause.Assignments(map[string]any{
			"visits":   gorm.Expr("tool_stats_daily.visits + EXCLUDED.visits"),
			"uses":     gorm.Expr("tool_stats_daily.uses + EXCLUDED.uses"),
			"failures": gorm.Expr("tool_stats_daily.failures + EXCLUDED.failures"),
		}),
	}).Create(&rows).Error
}

// UpsertResourceStats 批量 upsert，冲突时累加访问量。
func (s *GormStore) UpsertResourceStats(ctx context.Context, rows []domain.ResourceStatDaily) error {
	if len(rows) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "stat_date"}, {Name: "resource_slug"}},
		DoUpdates: clause.Assignments(map[string]any{
			"views": gorm.Expr("resource_stats_daily.views + EXCLUDED.views"),
		}),
	}).Create(&rows).Error
}

// UpsertAPIStats 批量 upsert，冲突时累加全部计数列（延迟分桶同样累加）。
func (s *GormStore) UpsertAPIStats(ctx context.Context, rows []domain.APIStatDaily) error {
	if len(rows) == 0 {
		return nil
	}
	cols := []string{
		"total", "errors", "client_errors", "rate_limited",
		"latency_ms_sum", "latency_le_50", "latency_le_200", "latency_le_500",
		"latency_le_1000", "latency_le_3000", "latency_gt_3000",
	}
	updates := make(map[string]any, len(cols)+1)
	for _, c := range cols {
		updates[c] = gorm.Expr("api_stats_daily." + c + " + EXCLUDED." + c)
	}
	// 最大耗时取两者较大值，不能累加。
	updates["latency_ms_max"] = gorm.Expr("GREATEST(api_stats_daily.latency_ms_max, EXCLUDED.latency_ms_max)")

	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "stat_date"}, {Name: "route"}, {Name: "method"}},
		DoUpdates: clause.Assignments(updates),
	}).Create(&rows).Error
}

// UpsertSearchStats 批量 upsert，冲突时累加搜索计数。
func (s *GormStore) UpsertSearchStats(ctx context.Context, rows []domain.SearchStatDaily) error {
	if len(rows) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "stat_date"}},
		DoUpdates: clause.Assignments(map[string]any{
			"queries":     gorm.Expr("search_stats_daily.queries + EXCLUDED.queries"),
			"zero_result": gorm.Expr("search_stats_daily.zero_result + EXCLUDED.zero_result"),
			"failed":      gorm.Expr("search_stats_daily.failed + EXCLUDED.failed"),
		}),
	}).Create(&rows).Error
}

// PruneBefore 删除超期日聚合行。
func (s *GormStore) PruneBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	if cutoff.IsZero() {
		return 0, nil
	}
	var total int64
	models := []any{
		&domain.ToolStatDaily{},
		&domain.ResourceStatDaily{},
		&domain.APIStatDaily{},
		&domain.SearchStatDaily{},
	}
	for _, m := range models {
		res := s.db.WithContext(ctx).Where("stat_date < ?", cutoff).Delete(m)
		if res.Error != nil {
			return total, res.Error
		}
		total += res.RowsAffected
	}
	return total, nil
}
