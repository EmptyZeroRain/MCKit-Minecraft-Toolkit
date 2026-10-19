package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/repository/types"
	"mckit/backend/internal/versioning"
)

// maxDevDocScan 开发文档版本筛选需在内存中精确判断（versioning.Satisfies），
// 故先取全集再分页；条目量级为数十条，此上限仅用于防御异常数据。
const maxDevDocScan = 2000

// DevDocFilter 开发文档查询筛选（TODO §21：按名称、分类、平台与游戏版本检索）。
type DevDocFilter struct {
	Q        string
	Loader   string
	Category string
	// Version 目标游戏版本（如 1.21.1）；为空表示不限版本。
	Version  string
	Page     int
	PageSize int
}

// DevDocSummary 列表条目（不含正文，避免列表响应过大）。
type DevDocSummary struct {
	Slug           string          `json:"slug"`
	TitleZh        string          `json:"title_zh"`
	TitleEn        string          `json:"title_en"`
	Loader         string          `json:"loader"`
	Category       string          `json:"category"`
	Summary        string          `json:"summary"`
	MCVersions     types.JSONSlice `json:"mc_versions"`
	Loaders        types.JSONSlice `json:"loaders"`
	Tags           types.JSONSlice `json:"tags"`
	APIEntryCount  int             `json:"api_entry_count"`
	OfficialURL    string          `json:"official_url"`
	SortOrder      int             `json:"sort_order"`
	LastVerifiedAt *time.Time      `json:"last_verified_at"`
}

// devDocSummaryColumns 列表查询字段：正文与来源 URL 不返回，接口数量用 SQL 计算。
const devDocSummaryColumns = `slug, title_zh, title_en, loader, category, summary,
	mc_versions, loaders, tags, official_url, sort_order, last_verified_at,
	jsonb_array_length(api_entries) AS api_entry_count`

// ListDevDocs 按关键词、平台、分类与适用版本查询开发文档。
//
// 版本筛选使用 versioning.Satisfies 的**精确语义**（选 1.21 命中 1.20.1+，
// 但不命中 1.22+），故命中结果需在内存中过滤后再分页；其余条件由数据库过滤。
func (s *Service) ListDevDocs(ctx context.Context, f DevDocFilter) ([]DevDocSummary, int64, error) {
	q := s.db.WithContext(ctx).Model(&domain.DevDoc{}).Where("status = ?", "published")

	if kw := strings.TrimSpace(f.Q); kw != "" {
		like := "%" + kw + "%"
		q = q.Where(
			`slug ILIKE ? OR title_zh ILIKE ? OR COALESCE(title_en,'') ILIKE ? OR summary ILIKE ?
			 OR tags::text ILIKE ? OR api_entries::text ILIKE ? OR body_md ILIKE ?`,
			like, like, like, like, like, like, like)
	}
	if loader := strings.TrimSpace(f.Loader); loader != "" {
		// loader 是主平台；loaders 是兼容平台集合（如 Fabric 主题也适用于 tooling 场景）。
		q = q.Where("(loader = ? OR loaders @> ?::jsonb)", loader, fmt.Sprintf(`[%q]`, loader))
	}
	if category := strings.TrimSpace(f.Category); category != "" {
		q = q.Where("category = ?", category)
	}

	target := strings.TrimSpace(f.Version)
	if target == "" || versioning.IsAll([]string{target}) {
		var total int64
		if err := q.Count(&total).Error; err != nil {
			return nil, 0, err
		}
		page, pageSize := normalizePage(f.Page, f.PageSize)
		items := make([]DevDocSummary, 0, pageSize)
		err := q.Select(devDocSummaryColumns).
			Order("sort_order ASC, slug ASC").
			Offset((page - 1) * pageSize).Limit(pageSize).
			Scan(&items).Error
		if err != nil {
			return nil, 0, err
		}
		return items, total, nil
	}

	var all []DevDocSummary
	err := q.Select(devDocSummaryColumns).Order("sort_order ASC, slug ASC").
		Limit(maxDevDocScan).Scan(&all).Error
	if err != nil {
		return nil, 0, err
	}

	matched := make([]DevDocSummary, 0, len(all))
	for _, row := range all {
		if versioning.Satisfies([]string(row.MCVersions), target) {
			matched = append(matched, row)
		}
	}
	total := int64(len(matched))
	page, pageSize := normalizePage(f.Page, f.PageSize)
	start := (page - 1) * pageSize
	if start >= len(matched) {
		return []DevDocSummary{}, total, nil
	}
	end := start + pageSize
	if end > len(matched) {
		end = len(matched)
	}
	return matched[start:end], total, nil
}

// DevDocFacet 一个筛选维度的取值与计数。
type DevDocFacet struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// DevDocFacets 开发文档筛选维度统计（平台 / 分类）。
// 前端据此渲染筛选项，避免把枚举硬编码在页面里。
type DevDocFacets struct {
	Loaders    []DevDocFacet `json:"loaders"`
	Categories []DevDocFacet `json:"categories"`
	Total      int64         `json:"total"`
}

// Facets 统计已发布开发文档的平台与分类分布。
func (s *Service) Facets(ctx context.Context) (*DevDocFacets, error) {
	out := &DevDocFacets{Loaders: []DevDocFacet{}, Categories: []DevDocFacet{}}
	base := s.db.WithContext(ctx).Model(&domain.DevDoc{}).Where("status = ?", "published")

	if err := base.Count(&out.Total).Error; err != nil {
		return nil, err
	}
	// Session(NewDB: true) 复用条件而不携带上一次的 Count 状态。
	if err := base.Session(&gorm.Session{}).
		Select("loader AS value, COUNT(*) AS count").
		Group("loader").Order("loader").Scan(&out.Loaders).Error; err != nil {
		return nil, err
	}
	if err := base.Session(&gorm.Session{}).
		Select("category AS value, COUNT(*) AS count").
		Group("category").Order("category").Scan(&out.Categories).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// GetDevDocBySlug 按 slug 获取开发文档详情（仅已发布）。
func (s *Service) GetDevDocBySlug(ctx context.Context, slug string) (*domain.DevDoc, error) {
	if strings.TrimSpace(slug) == "" {
		return nil, ErrNotFound
	}
	var doc domain.DevDoc
	err := s.db.WithContext(ctx).
		Where("slug = ? AND status = ?", slug, "published").
		First(&doc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

// RelatedDevDocs 返回同一平台的其它条目（详情页「继续阅读」）。
func (s *Service) RelatedDevDocs(ctx context.Context, doc *domain.DevDoc, limit int) ([]DevDocSummary, error) {
	if doc == nil {
		return []DevDocSummary{}, nil
	}
	if limit <= 0 || limit > 12 {
		limit = 6
	}
	items := make([]DevDocSummary, 0, limit)
	err := s.db.WithContext(ctx).Model(&domain.DevDoc{}).
		Where("status = ? AND loader = ? AND slug <> ?", "published", doc.Loader, doc.Slug).
		Select(devDocSummaryColumns).
		Order("sort_order ASC, slug ASC").
		Limit(limit).Scan(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}
