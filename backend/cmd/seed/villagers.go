// 村民交易资料导入（TODO §9B）。
//
// 数据文件为单个 JSON 对象：{ "professions": [...], "levels": [...], "trades": [...] }。
// 校验策略（对应 §9「校验资料来源和版本适用范围」）：
//   - profession：slug / name_zh / kind / supported_versions / source_url 为必填；
//     kind 必须属于 normal|wandering|non_trading；有工作站时须同时给出中文名。
//   - level：level 取 1–5、slug / name_zh 必填、required_experience ≥ 0。
//   - trade：trade_key / profession_slug / trade_group / result / supported_versions 必填；
//     profession_slug 必须能在同批数据或库中解析，否则视为悬空引用（致命）；
//     trade_group=level 时 level 必须为 1–5，其余分组 level 固定为 0；
//     cost_items 每项 item_id / name_zh 必填，count_min ≥ 1 且 count_max ≥ count_min。
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/repository/types"
	"mckit/backend/internal/versioning"
)

// villagerSeedFile 村民交易资料文件结构。
type villagerSeedFile struct {
	Professions []villagerProfessionRow `json:"professions"`
	Levels      []villagerLevelRow      `json:"levels"`
	Trades      []villagerTradeRow      `json:"trades"`
}

// villagerProfessionRow 村民职业导入原始行。
type villagerProfessionRow struct {
	Slug              string   `json:"slug"`
	NameZh            string   `json:"name_zh"`
	NameEn            string   `json:"name_en"`
	Aliases           []string `json:"aliases"`
	Kind              string   `json:"kind"`
	WorkstationItemID string   `json:"workstation_item_id"`
	WorkstationNameZh string   `json:"workstation_name_zh"`
	Summary           string   `json:"summary"`
	Description       string   `json:"description"`
	TradeNote         string   `json:"trade_note"`
	GameEdition       string   `json:"game_edition"`
	SupportedVersions []string `json:"supported_versions"`
	SourceURL         string   `json:"source_url"`
	DataVersion       string   `json:"data_version"`
	LastVerifiedAt    string   `json:"last_verified_at"`
	SortOrder         int      `json:"sort_order"`
}

// villagerLevelRow 职业等级导入原始行。
type villagerLevelRow struct {
	Level              int    `json:"level"`
	Slug               string `json:"slug"`
	NameZh             string `json:"name_zh"`
	NameEn             string `json:"name_en"`
	RequiredExperience int    `json:"required_experience"`
	BadgeNote          string `json:"badge_note"`
}

// villagerTradeRow 交易导入原始行。
type villagerTradeRow struct {
	TradeKey           string                    `json:"trade_key"`
	ProfessionSlug     string                    `json:"profession_slug"`
	Level              int                       `json:"level"`
	TradeGroup         string                    `json:"trade_group"`
	CostItems          []domain.VillagerCostItem `json:"cost_items"`
	ResultItemID       string                    `json:"result_item_id"`
	ResultItemNameZh   string                    `json:"result_item_name_zh"`
	ResultCount        int                       `json:"result_count"`
	MaxUses            int                       `json:"max_uses"`
	VillagerExperience int                       `json:"villager_experience"`
	PriceMultiplier    *float64                  `json:"price_multiplier"`
	VariantNote        string                    `json:"variant_note"`
	Note               string                    `json:"note"`
	GameEdition        string                    `json:"game_edition"`
	SupportedVersions  []string                  `json:"supported_versions"`
	SourceURL          string                    `json:"source_url"`
	SortOrder          int                       `json:"sort_order"`
}

// buildVillagerProfession 校验并归一化单个职业。
func buildVillagerProfession(row villagerProfessionRow) (*domain.VillagerProfession, error) {
	slug := strings.TrimSpace(row.Slug)
	nameZh := strings.TrimSpace(row.NameZh)
	if slug == "" || nameZh == "" {
		return nil, errors.New("slug 与 name_zh 必填")
	}
	kind := strings.TrimSpace(row.Kind)
	if kind == "" {
		kind = domain.VillagerKindNormal
	}
	if !domain.IsValidVillagerKind(kind) {
		return nil, fmt.Errorf("kind 非法: %q", row.Kind)
	}
	wsItem := strings.TrimSpace(row.WorkstationItemID)
	wsName := strings.TrimSpace(row.WorkstationNameZh)
	if wsItem != "" && wsName == "" {
		return nil, errors.New("有 workstation_item_id 时必须提供 workstation_name_zh")
	}
	sourceURL := strings.TrimSpace(row.SourceURL)
	if sourceURL == "" {
		return nil, errors.New("source_url 必填（需明确资料来源）")
	}
	versions := row.SupportedVersions
	if len(versions) == 0 {
		return nil, errors.New("supported_versions 必填（需明确版本适用范围）")
	}
	if err := validateVersions("supported_versions", versions); err != nil {
		return nil, err
	}
	edition := strings.TrimSpace(row.GameEdition)
	if edition == "" {
		edition = "java"
	}
	if !validEditions[edition] {
		return nil, fmt.Errorf("game_edition 非法: %q", row.GameEdition)
	}

	prof := &domain.VillagerProfession{
		Slug:              slug,
		NameZh:            nameZh,
		NameEn:            strings.TrimSpace(row.NameEn),
		Aliases:           types.JSONSlice(aliasOrEmpty(row.Aliases)),
		Kind:              kind,
		WorkstationItemID: wsItem,
		WorkstationNameZh: wsName,
		Summary:           strings.TrimSpace(row.Summary),
		Description:       strings.TrimSpace(row.Description),
		TradeNote:         strings.TrimSpace(row.TradeNote),
		GameEdition:       edition,
		SupportedVersions: types.JSONSlice(versioning.Sort(versions)),
		SourceURL:         sourceURL,
		DataVersion:       strings.TrimSpace(row.DataVersion),
		SortOrder:         row.SortOrder,
	}
	if raw := strings.TrimSpace(row.LastVerifiedAt); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, fmt.Errorf("last_verified_at 需为 RFC3339 时间: %v", err)
		}
		prof.LastVerifiedAt = &t
	}
	return prof, nil
}

// buildVillagerTradeLevel 校验并归一化单个职业等级。
func buildVillagerTradeLevel(row villagerLevelRow) (*domain.VillagerTradeLevel, error) {
	if row.Level < 1 || row.Level > 5 {
		return nil, fmt.Errorf("level 必须在 1–5 之间: %d", row.Level)
	}
	slug := strings.TrimSpace(row.Slug)
	nameZh := strings.TrimSpace(row.NameZh)
	if slug == "" || nameZh == "" {
		return nil, errors.New("slug 与 name_zh 必填")
	}
	if row.RequiredExperience < 0 {
		return nil, errors.New("required_experience 不能为负")
	}
	return &domain.VillagerTradeLevel{
		Level:              row.Level,
		Slug:               slug,
		NameZh:             nameZh,
		NameEn:             strings.TrimSpace(row.NameEn),
		RequiredExperience: row.RequiredExperience,
		BadgeNote:          strings.TrimSpace(row.BadgeNote),
	}, nil
}

// buildVillagerTrade 校验并归一化单条交易。
// knownProfessions 为同批数据中的职业 slug 集合，用于检出悬空引用。
func buildVillagerTrade(row villagerTradeRow, knownProfessions map[string]bool) (*domain.VillagerTrade, error) {
	key := strings.TrimSpace(row.TradeKey)
	if key == "" {
		return nil, errors.New("trade_key 必填")
	}
	profession := strings.TrimSpace(row.ProfessionSlug)
	if profession == "" {
		return nil, errors.New("profession_slug 必填")
	}
	if !knownProfessions[profession] {
		return nil, fmt.Errorf("profession_slug 未收录: %q", profession)
	}
	group := strings.TrimSpace(row.TradeGroup)
	if group == "" {
		group = domain.VillagerTradeGroupLevel
	}
	if !domain.IsValidVillagerTradeGroup(group) {
		return nil, fmt.Errorf("trade_group 非法: %q", row.TradeGroup)
	}
	// 等级约束：随等级解锁的交易 level 必须 1–5，其余分组固定 0。
	if group == domain.VillagerTradeGroupLevel {
		if row.Level < 1 || row.Level > 5 {
			return nil, fmt.Errorf("trade_group=level 时 level 必须在 1–5 之间: %d", row.Level)
		}
	} else if row.Level != 0 {
		return nil, fmt.Errorf("trade_group=%s 时 level 必须为 0", group)
	}

	costs := make(domain.VillagerCostItems, 0, len(row.CostItems))
	for i, c := range row.CostItems {
		itemID := strings.TrimSpace(c.ItemID)
		nameZh := strings.TrimSpace(c.NameZh)
		if itemID == "" || nameZh == "" {
			return nil, fmt.Errorf("cost_items[%d] 的 item_id 与 name_zh 必填", i)
		}
		if c.CountMin < 1 {
			return nil, fmt.Errorf("cost_items[%d].count_min 至少为 1", i)
		}
		countMax := c.CountMax
		if countMax == 0 {
			countMax = c.CountMin
		}
		if countMax < c.CountMin {
			return nil, fmt.Errorf("cost_items[%d].count_max 不能小于 count_min", i)
		}
		costs = append(costs, domain.VillagerCostItem{
			ItemID:   itemID,
			NameZh:   nameZh,
			CountMin: c.CountMin,
			CountMax: countMax,
		})
	}
	if len(costs) == 0 {
		return nil, errors.New("cost_items 至少需要一项（村民想要的物品）")
	}

	resultID := strings.TrimSpace(row.ResultItemID)
	if resultID == "" {
		return nil, errors.New("result_item_id 必填")
	}
	if strings.TrimSpace(row.ResultItemNameZh) == "" {
		return nil, errors.New("result_item_name_zh 必填")
	}
	resultCount := row.ResultCount
	if resultCount <= 0 {
		resultCount = 1
	}
	if row.MaxUses < 0 {
		return nil, errors.New("max_uses 不能为负")
	}
	if row.VillagerExperience < 0 {
		return nil, errors.New("villager_experience 不能为负")
	}
	priceMultiplier := 0.05
	if row.PriceMultiplier != nil {
		priceMultiplier = *row.PriceMultiplier
	}
	if priceMultiplier < 0 {
		return nil, errors.New("price_multiplier 不能为负")
	}

	versions := row.SupportedVersions
	if len(versions) == 0 {
		return nil, errors.New("supported_versions 必填")
	}
	if err := validateVersions("supported_versions", versions); err != nil {
		return nil, err
	}
	edition := strings.TrimSpace(row.GameEdition)
	if edition == "" {
		edition = "java"
	}
	if !validEditions[edition] {
		return nil, fmt.Errorf("game_edition 非法: %q", row.GameEdition)
	}
	sourceURL := strings.TrimSpace(row.SourceURL)
	if sourceURL == "" {
		sourceURL = villagerTradingSource
	}

	return &domain.VillagerTrade{
		TradeKey:           key,
		ProfessionSlug:     profession,
		Level:              row.Level,
		TradeGroup:         group,
		CostItems:          costs,
		ResultItemID:       resultID,
		ResultItemNameZh:   strings.TrimSpace(row.ResultItemNameZh),
		ResultCount:        resultCount,
		MaxUses:            row.MaxUses,
		VillagerExperience: row.VillagerExperience,
		PriceMultiplier:    priceMultiplier,
		VariantNote:        strings.TrimSpace(row.VariantNote),
		Note:               strings.TrimSpace(row.Note),
		GameEdition:        edition,
		SupportedVersions:  types.JSONSlice(versioning.Sort(versions)),
		SourceURL:          sourceURL,
		SortOrder:          row.SortOrder,
	}, nil
}

// villagerTradingSource 全部交易的默认来源页。
const villagerTradingSource = "https://minecraft.wiki/w/Trading"

// importVillagers 导入村民职业、职业等级与交易（先职业后交易，便于引用校验）。
func importVillagers(gormDB *gorm.DB, content []byte, dryRun bool) *summary {
	var file villagerSeedFile
	if err := json.Unmarshal(content, &file); err != nil {
		fmt.Fprintf(os.Stderr, "JSON 解析失败: %v\n", err)
		os.Exit(1)
	}

	s := &summary{total: len(file.Professions) + len(file.Levels) + len(file.Trades)}
	knownProfessions := make(map[string]bool, len(file.Professions))

	// ---- 职业 ----
	for i, row := range file.Professions {
		prof, err := buildVillagerProfession(row)
		if err != nil {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("职业第 %d 行：%v", i+1, err))
			continue
		}
		knownProfessions[prof.Slug] = true
		isNew, err := isNewRecord(gormDB, &domain.VillagerProfession{}, "slug = ?", prof.Slug)
		if err != nil {
			s.fail(fmt.Sprintf("%s 预查失败: %v", prof.Slug, err))
			continue
		}
		if dryRun {
			fmt.Printf("[dry-run] %s 职业 %s (%s)\n", upsertVerb(isNew), prof.Slug, prof.NameZh)
			s.countOutcome(isNew)
			continue
		}
		if err := gormDB.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "slug"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"name_zh", "name_en", "aliases", "kind", "workstation_item_id", "workstation_name_zh",
				"summary", "description", "trade_note", "game_edition", "supported_versions",
				"source_url", "data_version", "last_verified_at", "sort_order", "updated_at",
			}),
		}).Create(prof).Error; err != nil {
			s.fail(fmt.Sprintf("%s 导入失败: %v", prof.Slug, err))
			continue
		}
		s.countOutcome(isNew)
	}

	// ---- 职业等级 ----
	for i, row := range file.Levels {
		level, err := buildVillagerTradeLevel(row)
		if err != nil {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("等级第 %d 行：%v", i+1, err))
			continue
		}
		isNew, err := isNewRecord(gormDB, &domain.VillagerTradeLevel{}, "level = ?", level.Level)
		if err != nil {
			s.fail(fmt.Sprintf("等级 %d 预查失败: %v", level.Level, err))
			continue
		}
		if dryRun {
			fmt.Printf("[dry-run] %s 等级 %d (%s)\n", upsertVerb(isNew), level.Level, level.NameZh)
			s.countOutcome(isNew)
			continue
		}
		if err := gormDB.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "level"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"slug", "name_zh", "name_en", "required_experience", "badge_note", "updated_at",
			}),
		}).Create(level).Error; err != nil {
			s.fail(fmt.Sprintf("等级 %d 导入失败: %v", level.Level, err))
			continue
		}
		s.countOutcome(isNew)
	}

	// ---- 交易 ----
	for i, row := range file.Trades {
		trade, err := buildVillagerTrade(row, knownProfessions)
		if err != nil {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("交易第 %d 行：%v", i+1, err))
			continue
		}
		isNew, err := isNewRecord(gormDB, &domain.VillagerTrade{}, "trade_key = ?", trade.TradeKey)
		if err != nil {
			s.fail(fmt.Sprintf("%s 预查失败: %v", trade.TradeKey, err))
			continue
		}
		if dryRun {
			fmt.Printf("[dry-run] %s 交易 %s\n", upsertVerb(isNew), trade.TradeKey)
			s.countOutcome(isNew)
			continue
		}
		if err := gormDB.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "trade_key"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"profession_slug", "level", "trade_group", "cost_items", "result_item_id",
				"result_item_name_zh", "result_count", "max_uses", "villager_experience",
				"price_multiplier", "variant_note", "note", "game_edition", "supported_versions",
				"source_url", "sort_order", "updated_at",
			}),
		}).Create(trade).Error; err != nil {
			s.fail(fmt.Sprintf("%s 导入失败: %v", trade.TradeKey, err))
			continue
		}
		s.countOutcome(isNew)
	}
	return s
}
