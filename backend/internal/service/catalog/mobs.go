package catalog

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/versioning"
)

// maxMobScan 生物版本筛选需在内存中进行（精确版本语义），故先取全集再分页。
// 生物资料量级较小（数十条），此上限仅用于防御异常数据。
const maxMobScan = 500

// maxMobDrops 单个生物返回的掉落条数上限，避免异常数据导致无界返回。
const maxMobDrops = 200

// MobFilter 生物查询筛选（TODO §9C：按名称、类别与游戏版本检索）。
type MobFilter struct {
	Q        string
	Category string
	Version  string
	Page     int
	PageSize int
}

// ListMobs 按名称/别名、类别、适用版本查询生物。
//
// 版本筛选使用 versioning.Satisfies 的精确语义（1.20 可命中声明 1.14+，
// 但不会命中 1.21+），因此需在内存中过滤后再分页；关键词与类别仍由数据库过滤。
func (s *Service) ListMobs(ctx context.Context, f MobFilter) ([]domain.Mob, int64, error) {
	q := s.db.WithContext(ctx).Model(&domain.Mob{})
	if kw := strings.TrimSpace(f.Q); kw != "" {
		like := "%" + kw + "%"
		q = q.Where(
			"slug ILIKE ? OR name_zh ILIKE ? OR COALESCE(name_en,'') ILIKE ? OR aliases::text ILIKE ?",
			like, like, like, like,
		)
	}
	if category := strings.TrimSpace(f.Category); category != "" {
		q = q.Where("category = ?", category)
	}

	var all []domain.Mob
	if err := q.Order(mobCategoryOrder + ", sort_order ASC, name_zh ASC").
		Limit(maxMobScan).Find(&all).Error; err != nil {
		return nil, 0, err
	}

	target := strings.TrimSpace(f.Version)
	if target != "" {
		filtered := make([]domain.Mob, 0, len(all))
		for _, m := range all {
			if versioning.Satisfies(m.SupportedVersions, target) {
				filtered = append(filtered, m)
			}
		}
		all = filtered
	}

	total := int64(len(all))
	page, pageSize := normalizePage(f.Page, f.PageSize)
	start := (page - 1) * pageSize
	if start >= len(all) {
		return []domain.Mob{}, total, nil
	}
	end := start + pageSize
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], total, nil
}

// mobCategoryOrder 类别展示顺序（被动 → 中立 → 敌对 → Boss → 功能型 → 其他）。
const mobCategoryOrder = `CASE category
	WHEN 'passive' THEN 0
	WHEN 'neutral' THEN 1
	WHEN 'hostile' THEN 2
	WHEN 'boss' THEN 3
	WHEN 'utility' THEN 4
	ELSE 5 END`

// MobDropView 掉落展示视图：在原始掉落上补充**已收录**物品的摘要引用。
type MobDropView struct {
	domain.MobDrop
	// ItemRef 该掉落物在物品库中的摘要；未收录时为 nil，前端回退为纯文本 ID。
	ItemRef *ItemRef `json:"item_ref"`
}

// MobDetail 生物详情：本体 + 掉落 + 关联物品。
type MobDetail struct {
	domain.Mob
	// Drops 掉落清单（按排序号），已解析可跳转的物品引用。
	Drops []MobDropView `json:"drops"`
	// ResolvedItems 全部掉落引用到的、确实已收录的物品摘要（键为 item_id）。
	// 未收录物品不会出现在映射中，前端据此回退为纯文本 ID，避免断链。
	ResolvedItems map[string]ItemRef `json:"resolved_items"`
}

// GetMobBySlug 按 slug 查询生物详情（TODO §9C）。
func (s *Service) GetMobBySlug(ctx context.Context, slug string) (*MobDetail, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, ErrNotFound
	}
	var mob domain.Mob
	err := s.db.WithContext(ctx).Where("slug = ?", slug).First(&mob).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	detail := &MobDetail{
		Mob:           mob,
		Drops:         []MobDropView{},
		ResolvedItems: map[string]ItemRef{},
	}

	var drops []domain.MobDrop
	if err := s.db.WithContext(ctx).
		Where("mob_slug = ?", slug).
		Order("sort_order ASC, item_name_zh ASC").
		Limit(maxMobDrops).Find(&drops).Error; err != nil {
		return nil, err
	}

	itemIDs := make([]string, 0, len(drops))
	for _, d := range drops {
		itemIDs = append(itemIDs, d.ItemID)
	}
	refs, err := s.ResolveItemRefs(ctx, itemIDs)
	if err != nil {
		return nil, err
	}
	detail.ResolvedItems = refs

	for _, d := range drops {
		view := MobDropView{MobDrop: d}
		if ref, ok := refs[d.ItemID]; ok {
			refCopy := ref
			view.ItemRef = &refCopy
		}
		detail.Drops = append(detail.Drops, view)
	}
	return detail, nil
}
