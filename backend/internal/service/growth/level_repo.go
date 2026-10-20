package growth

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"mckit/backend/internal/domain"
)

// GetLevelConfig 读取等级曲线配置；缺失时返回默认曲线（不写库）。
func (r *Repo) GetLevelConfig(ctx context.Context) (*domain.GrowthLevelConfig, error) {
	var cfg domain.GrowthLevelConfig
	err := r.db.WithContext(ctx).Where("id = 1").First(&cfg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &domain.GrowthLevelConfig{
			ID: 1, MaxLevel: DefaultMaxLevel, StepXP: DefaultLevelStepXP,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ListLevelTitles 返回称号规则（按区间起点、终点升序）。
func (r *Repo) ListLevelTitles(ctx context.Context, enabledOnly bool) ([]domain.GrowthLevelTitle, error) {
	q := r.db.WithContext(ctx).Model(&domain.GrowthLevelTitle{})
	if enabledOnly {
		q = q.Where("enabled = true")
	}
	var out []domain.GrowthLevelTitle
	err := q.Order("min_level ASC, max_level ASC").Find(&out).Error
	return out, err
}

// loadLevelConfigTx 在事务内读取等级曲线。
func loadLevelConfigTx(tx *gorm.DB) (*domain.GrowthLevelConfig, error) {
	var cfg domain.GrowthLevelConfig
	err := tx.Where("id = 1").First(&cfg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &domain.GrowthLevelConfig{
			ID: 1, MaxLevel: DefaultMaxLevel, StepXP: DefaultLevelStepXP,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// loadLevelCurveTx 在事务内读取并归一化等级曲线。
func loadLevelCurveTx(tx *gorm.DB) (LevelCurve, error) {
	cfg, err := loadLevelConfigTx(tx)
	if err != nil {
		return LevelCurve{}, err
	}
	return NormalizeLevelCurve(cfg), nil
}

// loadLevelResolverTx 在事务内读取等级曲线与启用称号规则。
func loadLevelResolverTx(tx *gorm.DB) (LevelResolver, error) {
	cfg, err := loadLevelConfigTx(tx)
	if err != nil {
		return LevelResolver{}, err
	}
	var titles []domain.GrowthLevelTitle
	if err := tx.Where("enabled = true").
		Order("min_level ASC, max_level ASC").Find(&titles).Error; err != nil {
		return LevelResolver{}, err
	}
	return NewLevelResolver(cfg, titles), nil
}

// levelResolver 读取当前等级曲线与启用称号规则。
func (s *Service) levelResolver(ctx context.Context) (LevelResolver, error) {
	cfg, err := s.repo.GetLevelConfig(ctx)
	if err != nil {
		return LevelResolver{}, err
	}
	titles, err := s.repo.ListLevelTitles(ctx, true)
	if err != nil {
		return LevelResolver{}, err
	}
	return NewLevelResolver(cfg, titles), nil
}
