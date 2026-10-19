package catalog

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/versioning"
)

// maxRedstoneScan 红石资料的版本筛选需在内存中进行（精确版本语义），故先取全集再分页。
// 资料量级较小（数十条），此上限仅用于防御异常数据。
const maxRedstoneScan = 500

// maxRedstoneMaterials 单个设计返回的材料条数上限，避免异常数据导致无界返回。
const maxRedstoneMaterials = 200

// RedstoneComponentFilter 红石元件查询筛选（TODO §15：按名称、类别与游戏版本检索）。
type RedstoneComponentFilter struct {
	Query    string
	Category string
	Version  string
	Page     int
	PageSize int
}

// ListRedstoneComponents 按名称/别名、类别、适用版本查询红石元件。
//
// 版本筛选使用 versioning.Satisfies 的精确语义（1.20 可命中声明 1.14+，
// 但不会命中 1.21+），因此需在内存中过滤后再分页；关键词与类别仍由数据库过滤。
func (s *Service) ListRedstoneComponents(ctx context.Context, f RedstoneComponentFilter) ([]domain.RedstoneComponent, int64, error) {
	q := s.db.WithContext(ctx).Model(&domain.RedstoneComponent{})
	if kw := strings.TrimSpace(f.Query); kw != "" {
		like := "%" + kw + "%"
		q = q.Where(
			"slug ILIKE ? OR name_zh ILIKE ? OR COALESCE(name_en,'') ILIKE ? OR aliases::text ILIKE ?",
			like, like, like, like,
		)
	}
	if category := strings.TrimSpace(f.Category); category != "" {
		q = q.Where("category = ?", category)
	}

	var all []domain.RedstoneComponent
	if err := q.Order(redstoneCategoryOrder + ", sort_order ASC, name_zh ASC").
		Limit(maxRedstoneScan).Find(&all).Error; err != nil {
		return nil, 0, err
	}

	target := strings.TrimSpace(f.Version)
	if target != "" {
		filtered := make([]domain.RedstoneComponent, 0, len(all))
		for _, c := range all {
			if versioning.Satisfies(c.SupportedVersions, target) {
				filtered = append(filtered, c)
			}
		}
		all = filtered
	}

	total := int64(len(all))
	page, pageSize := normalizePage(f.Page, f.PageSize)
	start := (page - 1) * pageSize
	if start >= len(all) {
		return []domain.RedstoneComponent{}, total, nil
	}
	end := start + pageSize
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], total, nil
}

// redstoneCategoryOrder 元件类别展示顺序（信号源 → 传输 → 逻辑 → 执行 → 传感 → 其他）。
const redstoneCategoryOrder = `CASE category
	WHEN 'source' THEN 0
	WHEN 'transmission' THEN 1
	WHEN 'logic' THEN 2
	WHEN 'actuator' THEN 3
	WHEN 'sensor' THEN 4
	ELSE 5 END`

// GetRedstoneComponentBySlug 按 slug 查询红石元件详情（TODO §15）。
func (s *Service) GetRedstoneComponentBySlug(ctx context.Context, slug string) (*domain.RedstoneComponent, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, ErrNotFound
	}
	var component domain.RedstoneComponent
	err := s.db.WithContext(ctx).Where("slug = ?", slug).First(&component).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &component, nil
}

// RedstoneDesignFilter 机器设计查询筛选（TODO §15：按名称、用途、难度与游戏版本检索）。
type RedstoneDesignFilter struct {
	Query      string
	Purpose    string
	Difficulty string
	Version    string
	Page       int
	PageSize   int
}

// ListRedstoneDesigns 按名称/别名、用途、难度、适用版本查询**已发布**的机器设计。
//
// 只返回 review_status = published 的记录（内容审核流程：未审核内容不对公开接口可见）。
// 版本筛选同样使用 versioning.Satisfies 精确语义，需在内存中过滤后再分页。
func (s *Service) ListRedstoneDesigns(ctx context.Context, f RedstoneDesignFilter) ([]domain.RedstoneDesign, int64, error) {
	q := s.db.WithContext(ctx).Model(&domain.RedstoneDesign{}).
		Where("review_status = ?", domain.RedstoneReviewPublished)
	if kw := strings.TrimSpace(f.Query); kw != "" {
		like := "%" + kw + "%"
		q = q.Where(
			"slug ILIKE ? OR name_zh ILIKE ? OR COALESCE(name_en,'') ILIKE ? OR aliases::text ILIKE ? OR COALESCE(author,'') ILIKE ?",
			like, like, like, like, like,
		)
	}
	if purpose := strings.TrimSpace(f.Purpose); purpose != "" {
		q = q.Where("purpose = ?", purpose)
	}
	if difficulty := strings.TrimSpace(f.Difficulty); difficulty != "" {
		q = q.Where("difficulty = ?", difficulty)
	}

	var all []domain.RedstoneDesign
	if err := q.Order(redstonePurposeOrder + ", sort_order ASC, name_zh ASC").
		Limit(maxRedstoneScan).Find(&all).Error; err != nil {
		return nil, 0, err
	}

	target := strings.TrimSpace(f.Version)
	if target != "" {
		filtered := make([]domain.RedstoneDesign, 0, len(all))
		for _, d := range all {
			if versioning.Satisfies(d.SupportedVersions, target) {
				filtered = append(filtered, d)
			}
		}
		all = filtered
	}

	total := int64(len(all))
	page, pageSize := normalizePage(f.Page, f.PageSize)
	start := (page - 1) * pageSize
	if start >= len(all) {
		return []domain.RedstoneDesign{}, total, nil
	}
	end := start + pageSize
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], total, nil
}

// redstonePurposeOrder 设计用途展示顺序（农场 → 分类机 → 运输 → 门与机关 → 逻辑电路 → 实用装置 → 其他）。
const redstonePurposeOrder = `CASE purpose
	WHEN 'farm' THEN 0
	WHEN 'sorter' THEN 1
	WHEN 'transport' THEN 2
	WHEN 'door' THEN 3
	WHEN 'logic' THEN 4
	WHEN 'utility' THEN 5
	ELSE 6 END`

// RedstoneMaterialView 材料展示视图：在原始材料上补充**已收录**物品的摘要引用。
type RedstoneMaterialView struct {
	domain.RedstoneMaterial
	// ItemRef 该材料在物品库中的摘要；未收录（或 item_id 为空）时为 nil，前端回退为纯文本。
	ItemRef *ItemRef `json:"item_ref"`
}

// RedstoneDesignDetail 机器设计详情：本体 + 材料 + 关联物品。
type RedstoneDesignDetail struct {
	domain.RedstoneDesign
	// Materials 材料清单（按排序），已解析可跳转的物品引用。
	Materials []RedstoneMaterialView `json:"materials"`
	// ResolvedItems 全部材料引用到的、确实已收录的物品摘要（键为 item_id）。
	// 未收录物品不会出现在映射中，前端据此回退为纯文本，避免断链。
	ResolvedItems map[string]ItemRef `json:"resolved_items"`
}

// GetRedstoneDesignBySlug 按 slug 查询**已发布**的机器设计详情（TODO §15）。
func (s *Service) GetRedstoneDesignBySlug(ctx context.Context, slug string) (*RedstoneDesignDetail, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, ErrNotFound
	}
	var design domain.RedstoneDesign
	err := s.db.WithContext(ctx).
		Where("slug = ? AND review_status = ?", slug, domain.RedstoneReviewPublished).
		First(&design).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	detail := &RedstoneDesignDetail{
		RedstoneDesign: design,
		Materials:      []RedstoneMaterialView{},
		ResolvedItems:  map[string]ItemRef{},
	}

	materials := design.Materials
	if len(materials) > maxRedstoneMaterials {
		materials = materials[:maxRedstoneMaterials]
	}
	itemIDs := make([]string, 0, len(materials))
	for _, m := range materials {
		if m.ItemID != "" {
			itemIDs = append(itemIDs, m.ItemID)
		}
	}
	refs, err := s.ResolveItemRefs(ctx, itemIDs)
	if err != nil {
		return nil, err
	}
	detail.ResolvedItems = refs

	for _, m := range materials {
		view := RedstoneMaterialView{RedstoneMaterial: m}
		if ref, ok := refs[m.ItemID]; ok {
			refCopy := ref
			view.ItemRef = &refCopy
		}
		detail.Materials = append(detail.Materials, view)
	}
	return detail, nil
}

// RedstoneDesignAdminFilter 管理端设计列表筛选（TODO §15：按审核状态筛选）。
type RedstoneDesignAdminFilter struct {
	Query        string
	ReviewStatus string
	Purpose      string
	Page         int
	PageSize     int
}

// ListRedstoneDesignsForAdmin 管理端查询机器设计（不过滤发布态，供审核使用）。
func (s *Service) ListRedstoneDesignsForAdmin(ctx context.Context, f RedstoneDesignAdminFilter) ([]domain.RedstoneDesign, int64, error) {
	q := s.db.WithContext(ctx).Model(&domain.RedstoneDesign{})
	if status := strings.TrimSpace(f.ReviewStatus); status != "" {
		q = q.Where("review_status = ?", status)
	}
	if purpose := strings.TrimSpace(f.Purpose); purpose != "" {
		q = q.Where("purpose = ?", purpose)
	}
	if kw := strings.TrimSpace(f.Query); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("slug ILIKE ? OR name_zh ILIKE ? OR COALESCE(author,'') ILIKE ?", like, like, like)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, pageSize := normalizePage(f.Page, f.PageSize)
	var items []domain.RedstoneDesign
	err := q.Order("review_status ASC, sort_order ASC, name_zh ASC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ReviewRedstoneDesign 审核机器设计：更新审核状态并留痕（TODO §15 内容审核流程）。
// reviewer 取自登录会话；仅接受合法审核状态。
func (s *Service) ReviewRedstoneDesign(ctx context.Context, id, status, reviewer, note string) (*domain.RedstoneDesign, error) {
	status = strings.TrimSpace(status)
	if !domain.IsValidRedstoneReviewStatus(status) {
		return nil, ErrInvalidReviewStatus
	}
	if strings.TrimSpace(id) == "" {
		return nil, ErrNotFound
	}

	var design domain.RedstoneDesign
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&design).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	now := time.Now()
	updates := map[string]any{
		"review_status": status,
		"reviewer":      strings.TrimSpace(reviewer),
		"reviewed_at":   now,
		"review_note":   strings.TrimSpace(note),
		"updated_at":    now,
	}
	if err := s.db.WithContext(ctx).Model(&domain.RedstoneDesign{}).Where("id = ?", id).
		Updates(updates).Error; err != nil {
		return nil, err
	}
	design.ReviewStatus = status
	design.Reviewer = strings.TrimSpace(reviewer)
	design.ReviewedAt = &now
	design.ReviewNote = strings.TrimSpace(note)
	return &design, nil
}

// ErrInvalidReviewStatus 无效的审核状态。
var ErrInvalidReviewStatus = errors.New("无效的审核状态")
