// Package sitemap 汇总站点地图（sitemap.xml）中的动态内容页路径（需求 §9.3）。
// 仅收录「已发布且未软删除」的详情页；草稿、管理页、登录页不进入站点地图。
package sitemap

import (
	"context"
	"time"

	"gorm.io/gorm"

	"mckit/backend/internal/builtintools"
	"mckit/backend/internal/domain"
)

// maxEntriesPerType 单类型最大条目数：避免超大站点地图拖垮抓取与内存。
const maxEntriesPerType = 5000

// Entry 站点地图条目。
type Entry struct {
	Path    string
	LastMod time.Time
}

// Service 提供可索引的详情页路径。
type Service struct {
	db *gorm.DB
}

// NewService 创建站点地图服务。
func NewService(db *gorm.DB) *Service { return &Service{db: db} }

// builtinToolSlugs 内置工具 slug：它们各有专用落地页（前端路由重定向），
// 由 builtintools 单一清单收录；这里从动态 tools 中排除，避免重复收录。
var builtinToolSlugs = func() map[string]bool {
	m := make(map[string]bool, len(builtintools.Tools))
	for _, s := range builtintools.Slugs() {
		m[s] = true
	}
	return m
}()

// Entries 返回已发布内容的详情页路径（工具 / 资源 / 百科 / 物品）。
func (s *Service) Entries(ctx context.Context) ([]Entry, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	out := make([]Entry, 0, 128)

	tools, err := s.toolEntries(ctx)
	if err != nil {
		return nil, err
	}
	out = append(out, tools...)

	resources, err := s.resourceEntries(ctx)
	if err != nil {
		return nil, err
	}
	out = append(out, resources...)

	wiki, err := s.wikiEntries(ctx)
	if err != nil {
		return nil, err
	}
	out = append(out, wiki...)

	items, err := s.itemEntries(ctx)
	if err != nil {
		return nil, err
	}
	out = append(out, items...)

	potions, err := s.potionEntries(ctx)
	if err != nil {
		return nil, err
	}
	out = append(out, potions...)

	devDocs, err := s.devDocEntries(ctx)
	if err != nil {
		return nil, err
	}
	out = append(out, devDocs...)
	return out, nil
}

// slugRow 详情页 slug 与更新时间。
type slugRow struct {
	Slug      string
	UpdatedAt time.Time
}

func (s *Service) toolEntries(ctx context.Context) ([]Entry, error) {
	var rows []slugRow
	err := s.db.WithContext(ctx).Model(&domain.Tool{}).
		Select("slug", "updated_at").
		Where("status = ?", domain.StatusPublished).
		Order("updated_at DESC").
		Limit(maxEntriesPerType).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(rows))
	for _, r := range rows {
		if builtinToolSlugs[r.Slug] {
			continue
		}
		out = append(out, Entry{Path: "/tools/" + r.Slug, LastMod: r.UpdatedAt})
	}
	return out, nil
}

func (s *Service) resourceEntries(ctx context.Context) ([]Entry, error) {
	var rows []slugRow
	err := s.db.WithContext(ctx).Model(&domain.Resource{}).
		Select("slug", "updated_at").
		Where("status = ? AND deleted_at IS NULL", domain.StatusPublished).
		Order("updated_at DESC").
		Limit(maxEntriesPerType).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(rows))
	for _, r := range rows {
		out = append(out, Entry{Path: "/resources/" + r.Slug, LastMod: r.UpdatedAt})
	}
	return out, nil
}

func (s *Service) wikiEntries(ctx context.Context) ([]Entry, error) {
	var rows []slugRow
	err := s.db.WithContext(ctx).Model(&domain.WikiEntry{}).
		Select("slug", "updated_at").
		Where("status = ? AND deleted_at IS NULL", domain.StatusPublished).
		Order("updated_at DESC").
		Limit(maxEntriesPerType).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(rows))
	for _, r := range rows {
		out = append(out, Entry{Path: "/wiki/" + r.Slug, LastMod: r.UpdatedAt})
	}
	return out, nil
}

// itemEntries 物品详情页路径（统一物品资料库 TODO §1）。
// items 无草稿态，收录全部已收录物品；item_id 含冒号属合法路径字符。
func (s *Service) itemEntries(ctx context.Context) ([]Entry, error) {
	var rows []slugRow
	err := s.db.WithContext(ctx).Model(&domain.Item{}).
		Select("item_id AS slug", "updated_at").
		Order("updated_at DESC").
		Limit(maxEntriesPerType).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(rows))
	for _, r := range rows {
		out = append(out, Entry{Path: "/items/" + r.Slug, LastMod: r.UpdatedAt})
	}
	return out, nil
}

// potionEntries 药水详情页路径（TODO §9A）。药水无草稿态，收录全部已录入药水。
func (s *Service) potionEntries(ctx context.Context) ([]Entry, error) {
	var rows []slugRow
	err := s.db.WithContext(ctx).Model(&domain.Potion{}).
		Select("slug", "updated_at").
		Order("updated_at DESC").
		Limit(maxEntriesPerType).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(rows))
	for _, r := range rows {
		out = append(out, Entry{Path: "/potions/" + r.Slug, LastMod: r.UpdatedAt})
	}
	return out, nil
}

// devDocEntries 开发文档详情页路径（TODO §21）。草稿态不收录，与前台可见性一致。
func (s *Service) devDocEntries(ctx context.Context) ([]Entry, error) {
	var rows []slugRow
	err := s.db.WithContext(ctx).Model(&domain.DevDoc{}).
		Where("status = ?", "published").
		Select("slug", "updated_at").
		Order("updated_at DESC").
		Limit(maxEntriesPerType).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(rows))
	for _, r := range rows {
		out = append(out, Entry{Path: "/dev/" + r.Slug, LastMod: r.UpdatedAt})
	}
	return out, nil
}
