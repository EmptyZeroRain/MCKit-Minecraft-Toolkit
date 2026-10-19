package catalog

import (
	"context"
	"errors"
	"strings"

	"mckit/backend/internal/domain"
)

// ErrNoIDs 批量操作未提供任何 ID。
var ErrNoIDs = errors.New("请至少选择一条记录")

// ErrTooManyIDs 批量操作超过单次上限。
var ErrTooManyIDs = errors.New("单次批量操作条数超出上限")

// BatchReviewMods 批量审核 Mod：逐条独立执行，返回逐条结果。
func (s *Service) BatchReviewMods(
	ctx context.Context,
	ids []string,
	status, reviewer, note string,
) (*domain.BatchReviewResult, error) {
	return s.batchReview(ctx, ids, func(id string) (string, error) {
		m, err := s.ReviewMod(ctx, id, status, reviewer, note)
		if err != nil {
			return "", err
		}
		return m.NameZh, nil
	})
}

// BatchReviewModpacks 批量审核整合包：逐条独立执行，返回逐条结果。
func (s *Service) BatchReviewModpacks(
	ctx context.Context,
	ids []string,
	status, reviewer, note string,
) (*domain.BatchReviewResult, error) {
	return s.batchReview(ctx, ids, func(id string) (string, error) {
		p, err := s.ReviewModpack(ctx, id, status, reviewer, note)
		if err != nil {
			return "", err
		}
		return p.NameZh, nil
	})
}

// BatchReviewRedstoneDesigns 批量审核红石设计：逐条独立执行，返回逐条结果。
func (s *Service) BatchReviewRedstoneDesigns(
	ctx context.Context,
	ids []string,
	status, reviewer, note string,
) (*domain.BatchReviewResult, error) {
	return s.batchReview(ctx, ids, func(id string) (string, error) {
		d, err := s.ReviewRedstoneDesign(ctx, id, status, reviewer, note)
		if err != nil {
			return "", err
		}
		return d.NameZh, nil
	})
}

// batchReview 批量审核的公共骨架：去重 + 上限校验 + 逐条执行 + 错误脱敏。
//
// 单条错误统一转成可直接展示的中文文案；内部错误不外泄（避免暴露 SQL / 约束名）。
func (s *Service) batchReview(
	_ context.Context,
	ids []string,
	fn func(id string) (label string, err error),
) (*domain.BatchReviewResult, error) {
	ids = dedupeStrings(ids)
	if len(ids) == 0 {
		return nil, ErrNoIDs
	}
	if len(ids) > domain.MaxBatchReview {
		return nil, ErrTooManyIDs
	}

	result := &domain.BatchReviewResult{Items: make([]domain.BatchItemResult, 0, len(ids))}
	for _, id := range ids {
		item := domain.BatchItemResult{ID: id}
		label, err := fn(id)
		switch {
		case err == nil:
			item.OK = true
			item.Label = label
		case errors.Is(err, ErrNotFound):
			item.Error = "记录不存在"
		case errors.Is(err, ErrInvalidReviewStatus):
			item.Error = "审核状态非法"
		default:
			item.Error = "审核失败，请稍后重试"
		}
		result.Add(item)
	}
	return result, nil
}

// dedupeStrings 去重并保持原顺序；忽略空白项。
func dedupeStrings(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
