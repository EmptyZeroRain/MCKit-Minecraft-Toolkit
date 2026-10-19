package catalog

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/versioning"
)

// maxPotionScan 药水版本筛选需在内存中进行（精确版本语义），故先取全集再分页。
// 药水资料量级极小（数十条），此上限仅用于防御异常数据。
const maxPotionScan = 500

// maxBrewingSteps 单个方向返回的酿造步骤上限，避免异常数据导致无界返回。
const maxBrewingSteps = 50

// PotionFilter 药水查询筛选（TODO §9A：按名称、类别与游戏版本检索）。
type PotionFilter struct {
	Q        string
	Category string
	// Version 目标游戏版本（如 1.20 或 all）；为空表示不限版本。
	Version  string
	Page     int
	PageSize int
}

// ListPotions 按名称/别名、类别、适用版本查询药水。
//
// 版本筛选使用 versioning.Satisfies 的**精确语义**（1.20 可命中声明 1.8+，
// 但不会命中 1.21+），因此需在内存中过滤后再分页；关键词与类别仍由数据库过滤。
func (s *Service) ListPotions(ctx context.Context, f PotionFilter) ([]domain.Potion, int64, error) {
	q := s.db.WithContext(ctx).Model(&domain.Potion{})
	if kw := strings.TrimSpace(f.Q); kw != "" {
		like := "%" + kw + "%"
		q = q.Where(
			"slug ILIKE ? OR name_zh ILIKE ? OR COALESCE(name_en,'') ILIKE ? OR aliases::text ILIKE ?",
			like, like, like, like)
	}
	if c := strings.TrimSpace(f.Category); c != "" {
		q = q.Where("category = ?", c)
	}

	var all []domain.Potion
	if err := q.Order(potionCategoryOrder + ", name_zh ASC").Limit(maxPotionScan).Find(&all).Error; err != nil {
		return nil, 0, err
	}

	version := strings.TrimSpace(f.Version)
	filtered := make([]domain.Potion, 0, len(all))
	for _, p := range all {
		if version != "" && !versioning.Satisfies(p.SupportedVersions, version) {
			continue
		}
		filtered = append(filtered, p)
	}

	total := int64(len(filtered))
	page, pageSize := normalizePage(f.Page, f.PageSize)
	start := (page - 1) * pageSize
	if start >= len(filtered) {
		return []domain.Potion{}, total, nil
	}
	end := start + pageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total, nil
}

// potionCategoryOrder 类别展示顺序（水源 → 基础 → 增益 → 负面 → 混合 → 其他）。
const potionCategoryOrder = `CASE category
	WHEN 'water' THEN 0
	WHEN 'base' THEN 1
	WHEN 'positive' THEN 2
	WHEN 'negative' THEN 3
	WHEN 'mixed' THEN 4
	ELSE 5 END`

// PotionRef 关联展示用的药水摘要。
type PotionRef struct {
	Slug     string `json:"slug"`
	NameZh   string `json:"name_zh"`
	Category string `json:"category"`
	// URL 药水详情页路径；由后端统一生成，保证与列表/搜索的链接形态一致。
	URL string `json:"url"`
}

// BrewingStepView 酿造步骤展示视图：在原始步骤上补充可跳转的药水摘要。
type BrewingStepView struct {
	domain.BrewingRecipe
	InputPotion  *PotionRef `json:"input_potion"`
	ResultPotion *PotionRef `json:"result_potion"`
}

// PotionDetail 药水详情：本体 + 酿出步骤 + 后续步骤 + 通用规则 + 关联物品。
type PotionDetail struct {
	domain.Potion
	// ProducedBy 产出该药水的步骤（含基底药水与原料）。
	ProducedBy []BrewingStepView `json:"produced_by"`
	// FollowUps 以该药水为输入的后续步骤（腐化、延长、增强等）。
	FollowUps []BrewingStepView `json:"follow_ups"`
	// Rules 作用于任意适用药水的通用修饰 / 形态规则（与具体药水无关）。
	Rules []BrewingStepView `json:"rules"`
	// IngredientItems 上述步骤引用到的、**确实已收录**的物品摘要（键为 item_id）。
	// 未收录或已删除的物品不会出现在映射中，前端据此回退为纯文本 ID，避免断链。
	IngredientItems map[string]ItemRef `json:"ingredient_items"`
}

// GetPotionBySlug 按 slug 查询药水详情（TODO §9A）。
func (s *Service) GetPotionBySlug(ctx context.Context, slug string) (*PotionDetail, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, ErrNotFound
	}
	var potion domain.Potion
	err := s.db.WithContext(ctx).Where("slug = ?", slug).First(&potion).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	detail := &PotionDetail{
		Potion:          potion,
		ProducedBy:      []BrewingStepView{},
		FollowUps:       []BrewingStepView{},
		Rules:           []BrewingStepView{},
		IngredientItems: map[string]ItemRef{},
	}

	var produced, followUps, rules []domain.BrewingRecipe
	if err := s.db.WithContext(ctx).
		Where("result_potion_slug = ?", slug).
		Order("step_kind ASC, sort_order ASC, ingredient_name_zh ASC").
		Limit(maxBrewingSteps).Find(&produced).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).
		Where("input_potion_slug = ?", slug).
		Order("step_kind ASC, sort_order ASC, ingredient_name_zh ASC").
		Limit(maxBrewingSteps).Find(&followUps).Error; err != nil {
		return nil, err
	}
	// 通用规则：无输入亦无结果药水（红石延长、萤石增强、火药喷溅、龙息滞留）。
	if err := s.db.WithContext(ctx).
		Where("input_potion_slug IS NULL AND result_potion_slug IS NULL").
		Order("sort_order ASC").Limit(maxBrewingSteps).Find(&rules).Error; err != nil {
		return nil, err
	}

	// 汇总所有步骤引用的药水 slug 与原料物品，一次性解析摘要。
	slugs := make([]string, 0, 2*len(produced)+2*len(followUps))
	itemIDs := make([]string, 0, len(produced)+len(followUps)+len(rules))
	collect := func(steps []domain.BrewingRecipe) {
		for _, st := range steps {
			if st.InputPotionSlug != nil {
				slugs = append(slugs, *st.InputPotionSlug)
			}
			if st.ResultPotionSlug != nil {
				slugs = append(slugs, *st.ResultPotionSlug)
			}
			itemIDs = append(itemIDs, st.IngredientItemID)
		}
	}
	collect(produced)
	collect(followUps)
	collect(rules)

	potionRefs, err := s.resolvePotionRefs(ctx, slugs)
	if err != nil {
		return nil, err
	}
	itemRefs, err := s.ResolveItemRefs(ctx, itemIDs)
	if err != nil {
		return nil, err
	}

	toView := func(steps []domain.BrewingRecipe) []BrewingStepView {
		out := make([]BrewingStepView, 0, len(steps))
		for _, st := range steps {
			view := BrewingStepView{BrewingRecipe: st}
			if st.InputPotionSlug != nil {
				if ref, ok := potionRefs[*st.InputPotionSlug]; ok {
					view.InputPotion = &ref
				}
			}
			if st.ResultPotionSlug != nil {
				if ref, ok := potionRefs[*st.ResultPotionSlug]; ok {
					view.ResultPotion = &ref
				}
			}
			out = append(out, view)
		}
		return out
	}

	detail.ProducedBy = toView(produced)
	detail.FollowUps = toView(followUps)
	detail.Rules = toView(rules)
	detail.IngredientItems = itemRefs
	return detail, nil
}

// BrewingRules 通用酿造规则及其原料摘要。
type BrewingRules struct {
	Rules []BrewingStepView `json:"rules"`
	// IngredientItems 规则引用到的、确实已收录的物品摘要（键为 item_id）。
	IngredientItems map[string]ItemRef `json:"ingredient_items"`
}

// GenericBrewingRules 返回作用于任意药水的通用酿造规则（红石延长、萤石增强、火药喷溅、龙息滞留）。
// 供药水列表页展示"酿造规则"面板（规则存于数据库，前端不硬编码）。
func (s *Service) GenericBrewingRules(ctx context.Context) (*BrewingRules, error) {
	var rules []domain.BrewingRecipe
	if err := s.db.WithContext(ctx).
		Where("input_potion_slug IS NULL AND result_potion_slug IS NULL").
		Order("sort_order ASC").Limit(maxBrewingSteps).Find(&rules).Error; err != nil {
		return nil, err
	}
	itemIDs := make([]string, 0, len(rules))
	for _, r := range rules {
		itemIDs = append(itemIDs, r.IngredientItemID)
	}
	refs, err := s.ResolveItemRefs(ctx, itemIDs)
	if err != nil {
		return nil, err
	}
	out := &BrewingRules{Rules: make([]BrewingStepView, 0, len(rules)), IngredientItems: refs}
	for _, r := range rules {
		out.Rules = append(out.Rules, BrewingStepView{BrewingRecipe: r})
	}
	return out, nil
}

// resolvePotionRefs 批量解析药水摘要。
func (s *Service) resolvePotionRefs(ctx context.Context, slugs []string) (map[string]PotionRef, error) {
	out := make(map[string]PotionRef)
	seen := make(map[string]bool, len(slugs))
	unique := make([]string, 0, len(slugs))
	for _, slug := range slugs {
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		unique = append(unique, slug)
	}
	if len(unique) == 0 {
		return out, nil
	}
	var potions []domain.Potion
	if err := s.db.WithContext(ctx).
		Select("slug", "name_zh", "category").
		Where("slug IN ?", unique).Find(&potions).Error; err != nil {
		return nil, err
	}
	for _, p := range potions {
		out[p.Slug] = PotionRef{
			Slug:     p.Slug,
			NameZh:   p.NameZh,
			Category: p.Category,
			URL:      "/potions/" + p.Slug,
		}
	}
	return out, nil
}
