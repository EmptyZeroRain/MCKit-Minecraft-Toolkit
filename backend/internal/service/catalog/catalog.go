// Package catalog 提供物品/配方/指令的数据查询（需求 §5.4、§5.5、§5.6）。
package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/repository/types"
)

// ErrNotFound 记录不存在。
var ErrNotFound = errors.New("数据不存在")

// Service 目录数据服务。
type Service struct {
	db *gorm.DB
}

// NewService 创建目录服务。
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// ---- 物品 ----

// ItemFilter 物品查询筛选。
type ItemFilter struct {
	Q        string
	Page     int
	PageSize int
}

// ListItems 按中文名、英文名、标准 ID、别名查询物品。
func (s *Service) ListItems(ctx context.Context, f ItemFilter) ([]domain.Item, int64, error) {
	q := s.db.WithContext(ctx).Model(&domain.Item{})
	if f.Q != "" {
		kw := "%" + strings.TrimSpace(f.Q) + "%"
		q = q.Where(
			"item_id ILIKE ? OR name_zh ILIKE ? OR COALESCE(name_en,'') ILIKE ? OR aliases::text ILIKE ?",
			kw, kw, kw, kw)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, pageSize := normalizePage(f.Page, f.PageSize)
	var items []domain.Item
	err := q.Order("name_zh ASC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// GetItemByID 按标准 ID（如 minecraft:stone）查询物品。
// path 参数可能含冒号，使用 URL 编码后由路由层还原。
func (s *Service) GetItemByID(ctx context.Context, itemID string) (*domain.Item, error) {
	var item domain.Item
	err := s.db.WithContext(ctx).Where("item_id = ?", itemID).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// maxRelatedRecipes 单个方向（产出 / 作为材料）关联配方的上限，避免详情页无界返回。
const maxRelatedRecipes = 20

// maxResolveItems 单次关联解析的物品数上限，避免异常数据导致超大 IN 查询。
const maxResolveItems = 200

// ItemRef 关联展示用的物品摘要（只含跳转与展示所需字段）。
type ItemRef struct {
	ItemID       string `json:"item_id"`
	NameZh       string `json:"name_zh"`
	NameEn       string `json:"name_en"`
	ItemCategory string `json:"item_category"`
	// URL 物品详情页路径；由后端统一生成，保证与搜索结果的链接形态一致。
	URL string `json:"url"`
}

// ItemRecipeLinks 物品关联配方：产出该物品、以及以该物品为材料。
type ItemRecipeLinks struct {
	Produced []domain.Recipe `json:"produced"`
	UsedAs   []domain.Recipe `json:"used_as"`
	// ResolvedItems 上述配方引用到的、**确实存在于物品库**的物品摘要（键为 item_id）。
	// 未收录或已删除的物品不会出现在映射中，前端据此回退为纯文本 ID，避免产生断链。
	ResolvedItems map[string]ItemRef `json:"resolved_items"`
}

// RelatedItemRecipes 返回与物品关联的配方（TODO §1「物品详情与配方互相关联」）。
// 关联基于可靠元数据：result_item_id 精确匹配 + ingredients JSONB 包含匹配；
// 不会牵连已删除或未发布内容（配方表无发布态，均为已收录数据）。
func (s *Service) RelatedItemRecipes(ctx context.Context, itemID string) (*ItemRecipeLinks, error) {
	if itemID == "" {
		return nil, ErrNotFound
	}
	out := &ItemRecipeLinks{
		Produced:      []domain.Recipe{},
		UsedAs:        []domain.Recipe{},
		ResolvedItems: map[string]ItemRef{},
	}

	if err := s.db.WithContext(ctx).
		Where("result_item_id = ?", itemID).
		Order("game_version DESC, recipe_type ASC").
		Limit(maxRelatedRecipes).
		Find(&out.Produced).Error; err != nil {
		return nil, err
	}

	// ingredients 为 [{item_id,count}, ...]，用 JSONB 包含匹配避免子串误命中；
	// jsonb_build_object 通过参数绑定，杜绝注入。
	if err := s.db.WithContext(ctx).
		Where("ingredients @> jsonb_build_array(jsonb_build_object('item_id', ?::text))", itemID).
		Order("game_version DESC, recipe_type ASC").
		Limit(maxRelatedRecipes).
		Find(&out.UsedAs).Error; err != nil {
		return nil, err
	}

	// TODO §3：把关联配方引用到的物品一次性解析为可跳转摘要（有效性检查见 ResolveItemRefs）。
	refs, err := s.ResolveItemRefs(ctx, RecipeItems(out.Produced, out.UsedAs))
	if err != nil {
		return nil, err
	}
	out.ResolvedItems = refs
	return out, nil
}

// RecipeItems 汇总一批配方引用到的全部 item_id（去重，结果 + 材料）。
// 供批量解析关联物品使用（TODO §3）。
func RecipeItems(recipes ...[]domain.Recipe) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	add := func(id string) {
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, group := range recipes {
		for _, r := range group {
			add(r.ResultItemID)
			for _, id := range ingredientItemIDs(r.Ingredients) {
				add(id)
			}
		}
	}
	return out
}

// RecipeRefItems 单个配方引用到的 item_id（结果 + 材料），用于按配方切分解析结果。
func RecipeRefItems(r domain.Recipe) []string {
	return RecipeItems([]domain.Recipe{r})
}

// ingredientItemIDs 从 ingredients JSONB 提取 item_id（结构：[{item_id,count}]）。
// 解析失败按「无材料」处理：材料是展示信息，不应让整个查询失败。
func ingredientItemIDs(raw types.JSONB) []string {
	if len(raw) == 0 {
		return nil
	}
	var rows []struct {
		ItemID string `json:"item_id"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.ItemID != "" {
			out = append(out, r.ItemID)
		}
	}
	return out
}

// ResolveItemRefs 批量解析物品摘要（TODO §3 关联有效性检查）。
//
// **只返回 items 表中真实存在的物品**：引用未收录物品时该键不会出现，
// 调用方必须回退为纯文本 ID，绝不生成指向不存在详情页的链接。
// 物品表无发布态与软删列（物品为公开资料库），因此无需额外状态过滤。
func (s *Service) ResolveItemRefs(ctx context.Context, itemIDs []string) (map[string]ItemRef, error) {
	out := make(map[string]ItemRef)
	if len(itemIDs) == 0 {
		return out, nil
	}
	ids := itemIDs
	if len(ids) > maxResolveItems {
		ids = ids[:maxResolveItems]
	}
	var items []domain.Item
	if err := s.db.WithContext(ctx).Where("item_id IN ?", ids).Find(&items).Error; err != nil {
		return nil, err
	}
	for _, it := range items {
		out[it.ItemID] = ItemRef{
			ItemID:       it.ItemID,
			NameZh:       it.NameZh,
			NameEn:       it.NameEn,
			ItemCategory: it.ItemCategory,
			URL:          "/items/" + it.ItemID,
		}
	}
	return out, nil
}

// ---- 配方 ----

// RecipeFilter 配方查询筛选（需求 §8.5）。
type RecipeFilter struct {
	ItemID      string
	GameVersion string
	RecipeType  string
	Page        int
	PageSize    int
}

// ListRecipes 按产出物品/版本/配方类型查询配方。
// item_id 建议必填（需求 §8.5）；game_version 用于精确版本过滤。
func (s *Service) ListRecipes(ctx context.Context, f RecipeFilter) ([]domain.Recipe, int64, error) {
	q := s.db.WithContext(ctx).Model(&domain.Recipe{})
	if f.ItemID != "" {
		q = q.Where("result_item_id = ?", f.ItemID)
	}
	if f.GameVersion != "" {
		q = q.Where("game_version = ?", f.GameVersion)
	}
	if f.RecipeType != "" {
		q = q.Where("recipe_type = ?", f.RecipeType)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, pageSize := normalizePage(f.Page, f.PageSize)
	var items []domain.Recipe
	err := q.Order("game_version DESC, recipe_type ASC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ---- 指令 ----

// CommandFilter 指令列表筛选。
type CommandFilter struct {
	Q        string
	Status   string
	Page     int
	PageSize int
}

// ListCommands 指令列表与搜索（需求 §5.6）。
func (s *Service) ListCommands(ctx context.Context, f CommandFilter) ([]domain.Command, int64, error) {
	q := s.db.WithContext(ctx).Model(&domain.Command{})
	if f.Status == "" {
		f.Status = domain.StatusPublished
	}
	q = q.Where("status = ?", f.Status)
	if f.Q != "" {
		kw := "%" + strings.TrimSpace(f.Q) + "%"
		q = q.Where("name ILIKE ? OR slug ILIKE ? OR description ILIKE ?", kw, kw, kw)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, pageSize := normalizePage(f.Page, f.PageSize)
	var items []domain.Command
	err := q.Order("name ASC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// GetCommandBySlug 指令详情。
func (s *Service) GetCommandBySlug(ctx context.Context, slug string) (*domain.Command, error) {
	var cmd domain.Command
	err := s.db.WithContext(ctx).
		Where("slug = ? AND status = ?", slug, domain.StatusPublished).
		First(&cmd).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &cmd, nil
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 50 {
		pageSize = 50
	}
	return page, pageSize
}
