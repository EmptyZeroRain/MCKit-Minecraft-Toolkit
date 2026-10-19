package resource

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/repository/types"
)

// Detail 资源详情（资源本体 + 发布版本 + 兼容性结论，TODO §6）。
type Detail struct {
	domain.Resource
	Releases      []domain.ResourceRelease `json:"releases"`
	Compatibility Compatibility            `json:"compatibility"`
}

// GetDetailBySlug 公开详情：仅已发布资源，附带发布版本与兼容性结论。
func (s *Service) GetDetailBySlug(ctx context.Context, slug string) (*Detail, error) {
	res, err := s.GetBySlug(ctx, slug, true)
	if err != nil {
		return nil, err
	}
	releases, err := s.ListReleases(ctx, res.ID)
	if err != nil {
		return nil, err
	}
	return &Detail{
		Resource:      *res,
		Releases:      releases,
		Compatibility: BuildCompatibility(res, releases),
	}, nil
}

// GetDetailByID 管理端详情：附带发布版本与兼容性结论（TODO §6）。
func (s *Service) GetDetailByID(ctx context.Context, id string) (*Detail, error) {
	res, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	releases, err := s.ListReleases(ctx, res.ID)
	if err != nil {
		return nil, err
	}
	return &Detail{
		Resource:      *res,
		Releases:      releases,
		Compatibility: BuildCompatibility(res, releases),
	}, nil
}

// ListReleases 查询某资源的全部发布版本。
// 排序：发布时间降序（未知时间排后），同一时间按版本标签升序。
func (s *Service) ListReleases(ctx context.Context, resourceID uuid.UUID) ([]domain.ResourceRelease, error) {
	out := make([]domain.ResourceRelease, 0)
	err := s.db.WithContext(ctx).
		Where("resource_id = ?", resourceID).
		Order("released_at DESC NULLS LAST, version_label ASC").
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ReplaceReleases 整体替换某资源的发布版本（事务内删除 + 插入）。
//
// 采用「整体替换」而非增量合并：导入与后台表单都以完整列表为准，
// 删除后重建可避免残留陈旧版本，天然幂等。
func (s *Service) ReplaceReleases(ctx context.Context, resourceID uuid.UUID, items []ReleaseInput) ([]domain.ResourceRelease, error) {
	normalized, err := NormalizeReleases(items)
	if err != nil {
		return nil, err
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("resource_id = ?", resourceID).
			Delete(&domain.ResourceRelease{}).Error; err != nil {
			return err
		}
		for _, it := range normalized {
			rel := &domain.ResourceRelease{
				ResourceID:   resourceID,
				VersionLabel: it.VersionLabel,
				GameVersions: types.JSONSlice(it.GameVersions),
				LoaderTypes:  types.JSONSlice(it.LoaderTypes),
				SourceURL:    it.SourceURL,
				Notes:        it.Notes,
				IsLatest:     it.IsLatest,
			}
			if it.ReleasedAt != "" {
				t, perr := ParseReleaseDate(it.ReleasedAt)
				if perr != nil {
					return perr
				}
				rel.ReleasedAt = &t
			}
			if err := tx.Create(rel).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.ListReleases(ctx, resourceID)
}
