package catalog

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/versioning"
)

// maxVillagerScan 村民职业版本筛选需在内存中进行（精确版本语义），故先取全集再分页。
// 职业资料量级极小（十余条），此上限仅用于防御异常数据。
const maxVillagerScan = 200

// maxVillagerTrades 单个职业返回的交易条数上限，避免异常数据导致无界返回。
const maxVillagerTrades = 500

// VillagerFilter 村民职业查询筛选（TODO §9B：按名称、类别与游戏版本检索）。
type VillagerFilter struct {
	Q        string
	Kind     string
	Version  string
	Page     int
	PageSize int
}

// ListVillagerProfessions 按名称/别名、职业类别、适用版本查询村民职业。
//
// 版本筛选使用 versioning.Satisfies 的精确语义（1.20 可命中声明 1.14+，
// 但不会命中 1.21+），因此需在内存中过滤后再分页；关键词与类别仍由数据库过滤。
func (s *Service) ListVillagerProfessions(ctx context.Context, f VillagerFilter) ([]domain.VillagerProfession, int64, error) {
	q := s.db.WithContext(ctx).Model(&domain.VillagerProfession{})
	if kw := strings.TrimSpace(f.Q); kw != "" {
		like := "%" + kw + "%"
		q = q.Where(
			"slug ILIKE ? OR name_zh ILIKE ? OR COALESCE(name_en,'') ILIKE ? OR aliases::text ILIKE ? OR workstation_name_zh ILIKE ?",
			like, like, like, like, like,
		)
	}
	if kind := strings.TrimSpace(f.Kind); kind != "" {
		q = q.Where("kind = ?", kind)
	}

	var all []domain.VillagerProfession
	if err := q.Order(villagerKindOrder + ", sort_order ASC, name_zh ASC").
		Limit(maxVillagerScan).Find(&all).Error; err != nil {
		return nil, 0, err
	}

	target := strings.TrimSpace(f.Version)
	if target != "" {
		filtered := make([]domain.VillagerProfession, 0, len(all))
		for _, p := range all {
			if versioning.Satisfies(p.SupportedVersions, target) {
				filtered = append(filtered, p)
			}
		}
		all = filtered
	}

	total := int64(len(all))
	page, pageSize := normalizePage(f.Page, f.PageSize)
	start := (page - 1) * pageSize
	if start >= len(all) {
		return []domain.VillagerProfession{}, total, nil
	}
	end := start + pageSize
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], total, nil
}

// villagerKindOrder 类别展示顺序（可交易职业 → 流浪商人 → 不参与交易）。
const villagerKindOrder = `CASE kind
	WHEN 'normal' THEN 0
	WHEN 'wandering' THEN 1
	ELSE 2 END`

// VillagerTradeLevels 返回全部职业等级定义（新手 → 大师），供列表/详情页展示等级规则。
func (s *Service) VillagerTradeLevels(ctx context.Context) ([]domain.VillagerTradeLevel, error) {
	var levels []domain.VillagerTradeLevel
	if err := s.db.WithContext(ctx).Order("level ASC").Find(&levels).Error; err != nil {
		return nil, err
	}
	return levels, nil
}

// VillagerTradeView 交易展示视图：在原始交易上补充成本物品的摘要引用。
type VillagerTradeView struct {
	domain.VillagerTrade
	// CostItemRefs 本条交易成本物品中**确实已收录**的物品摘要（键为 item_id）。
	CostItemRefs map[string]ItemRef `json:"cost_item_refs"`
}

// VillagerTradeGroup 按（等级 / 分组）聚合的交易集合。
// 普通职业按 1–5 级分组（trade_group=level）；流浪商人按 收购/特殊/普通 分组（level=0）。
type VillagerTradeGroup struct {
	Level              int                 `json:"level"`
	LevelSlug          string              `json:"level_slug"`
	LevelNameZh        string              `json:"level_name_zh"`
	RequiredExperience int                 `json:"required_experience"`
	TradeGroup         string              `json:"trade_group"`
	Trades             []VillagerTradeView `json:"trades"`
}

// VillagerProfessionDetail 村民职业详情：本体 + 等级规则 + 分组交易 + 关联物品。
type VillagerProfessionDetail struct {
	domain.VillagerProfession
	// Levels 全部职业等级定义（通用规则），供前端渲染等级说明。
	Levels []domain.VillagerTradeLevel `json:"levels"`
	// Groups 分组后的交易（普通职业按等级，流浪商人按收购/特殊/普通）。
	Groups []VillagerTradeGroup `json:"groups"`
	// Trades 扁平交易列表（按等级 / 排序号），便于一次性渲染或统计。
	Trades []VillagerTradeView `json:"trades"`
	// WorkstationItem 工作站方块摘要；未收录或无工作站时为 nil。
	WorkstationItem *ItemRef `json:"workstation_item"`
	// ResolvedItems 全部交易引用到的、确实已收录的物品摘要（键为 item_id）。
	// 未收录物品不会出现在映射中，前端据此回退为纯文本 ID，避免断链。
	ResolvedItems map[string]ItemRef `json:"resolved_items"`
}

// GetVillagerProfessionBySlug 按 slug 查询村民职业详情（TODO §9B）。
func (s *Service) GetVillagerProfessionBySlug(ctx context.Context, slug string) (*VillagerProfessionDetail, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, ErrNotFound
	}
	var prof domain.VillagerProfession
	err := s.db.WithContext(ctx).Where("slug = ?", slug).First(&prof).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	detail := &VillagerProfessionDetail{
		VillagerProfession: prof,
		Levels:             []domain.VillagerTradeLevel{},
		Groups:             []VillagerTradeGroup{},
		Trades:             []VillagerTradeView{},
		ResolvedItems:      map[string]ItemRef{},
	}

	levels, err := s.VillagerTradeLevels(ctx)
	if err != nil {
		return nil, err
	}
	detail.Levels = levels

	var trades []domain.VillagerTrade
	if err := s.db.WithContext(ctx).
		Where("profession_slug = ?", slug).
		Order("level ASC, trade_group ASC, sort_order ASC, result_item_name_zh ASC").
		Limit(maxVillagerTrades).Find(&trades).Error; err != nil {
		return nil, err
	}

	// 汇总所有交易引用的物品 ID（成本 + 结果 + 工作站），一次性解析摘要。
	itemIDs := make([]string, 0, 2*len(trades)+1)
	if prof.WorkstationItemID != "" {
		itemIDs = append(itemIDs, prof.WorkstationItemID)
	}
	for _, t := range trades {
		itemIDs = append(itemIDs, t.ResultItemID)
		for _, c := range t.CostItems {
			itemIDs = append(itemIDs, c.ItemID)
		}
	}
	refs, err := s.ResolveItemRefs(ctx, itemIDs)
	if err != nil {
		return nil, err
	}
	detail.ResolvedItems = refs
	if prof.WorkstationItemID != "" {
		if ref, ok := refs[prof.WorkstationItemID]; ok {
			detail.WorkstationItem = &ref
		}
	}

	// 构造视图并按（等级 / 分组）聚合，保持首次出现顺序。
	levelMeta := make(map[int]domain.VillagerTradeLevel, len(levels))
	for _, l := range levels {
		levelMeta[l.Level] = l
	}
	groupIndex := map[string]int{}
	views := make([]VillagerTradeView, 0, len(trades))
	for _, t := range trades {
		view := VillagerTradeView{VillagerTrade: t, CostItemRefs: map[string]ItemRef{}}
		for _, c := range t.CostItems {
			if ref, ok := refs[c.ItemID]; ok {
				view.CostItemRefs[c.ItemID] = ref
			}
		}
		views = append(views, view)

		key := groupKey(t.Level, t.TradeGroup)
		idx, ok := groupIndex[key]
		if !ok {
			meta := levelMeta[t.Level]
			detail.Groups = append(detail.Groups, VillagerTradeGroup{
				Level:              t.Level,
				LevelSlug:          meta.Slug,
				LevelNameZh:        meta.NameZh,
				RequiredExperience: meta.RequiredExperience,
				TradeGroup:         t.TradeGroup,
				Trades:             []VillagerTradeView{},
			})
			idx = len(detail.Groups) - 1
			groupIndex[key] = idx
		}
		detail.Groups[idx].Trades = append(detail.Groups[idx].Trades, view)
	}
	detail.Trades = views
	return detail, nil
}

// groupKey 交易聚合键（等级 + 分组）。
func groupKey(level int, group string) string {
	return group + "#" + strconv.Itoa(level)
}
