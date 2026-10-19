// Package dataquality 提供资料库数据质量检查与导入任务查询（TODO §4）。
//
// 设计原则：
//   - 只做「可验证的客观检查」（缺失、非法格式、悬空引用），不做主观质量评分；
//   - 严重（error）表示数据不一致或引用断裂；提醒（warning）表示字段缺失，
//     历史数据可能尚未补齐，不应阻断使用；
//   - 唯一性约束由数据库保证（item_id / recipe_key / slug），此处不重复校验。
package dataquality

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/repository/types"
	"mckit/backend/internal/versioning"
)

// 检查严重级别。
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

// Check 单项数据质量检查结果。
type Check struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Category string `json:"category"` // items | recipes | commands | potions | dev_docs | import
	Severity string `json:"severity"` // error | warning | info
	Count    int64  `json:"count"`    // 命中问题的条目数
	Total    int64  `json:"total"`    // 该表总条目数（便于展示占比）
	Hint     string `json:"hint"`
}

// Overview 数据质量概览。
type Overview struct {
	GeneratedAt   time.Time `json:"generated_at"`
	Checks        []Check   `json:"checks"`
	ErrorChecks   int       `json:"error_checks"`   // 存在严重问题的检查项数
	WarningChecks int       `json:"warning_checks"` // 存在提醒的检查项数
}

// Service 数据质量服务。
type Service struct {
	db *gorm.DB
}

// NewService 创建数据质量服务。
func NewService(db *gorm.DB) *Service { return &Service{db: db} }

func (o *Overview) add(c Check) {
	if c.Count > 0 {
		switch c.Severity {
		case SeverityError:
			o.ErrorChecks++
		case SeverityWarning:
			o.WarningChecks++
		}
	}
	o.Checks = append(o.Checks, c)
}

// Overview 汇总数据质量检查结果。
func (s *Service) Overview(ctx context.Context) (*Overview, error) {
	db := s.db.WithContext(ctx)
	out := &Overview{GeneratedAt: time.Now(), Checks: make([]Check, 0, 16)}

	// ---- 物品 ----
	itemsTotal, err := s.count(db, &domain.Item{}, "", nil)
	if err != nil {
		return nil, err
	}
	out.add(Check{Key: "items_total", Label: "物品总数", Category: "items",
		Severity: SeverityInfo, Count: itemsTotal, Total: itemsTotal, Hint: "统一物品资料库条目数。"})

	for _, c := range []struct {
		key, label, where, hint string
	}{
		{"items_missing_source", "缺少来源 URL", "source_url IS NULL OR source_url = ''", "补齐 source_url 以便溯源（TODO §1）。"},
		{"items_missing_versions", "缺少适用版本", "supported_versions = '[]'::jsonb", "补齐 supported_versions，避免版本适用性不清。"},
		{"items_unverified", "超过 365 天未校验", "last_verified_at IS NULL OR last_verified_at < now() - interval '365 days'", "复核资料并更新 last_verified_at。"},
	} {
		n, err := s.count(db, &domain.Item{}, c.where, nil)
		if err != nil {
			return nil, err
		}
		out.add(Check{Key: c.key, Label: c.label, Category: "items",
			Severity: SeverityWarning, Count: n, Total: itemsTotal, Hint: c.hint})
	}
	if n, err := s.invalidVersions(ctx, &domain.Item{}, "supported_versions"); err != nil {
		return nil, err
	} else {
		out.add(Check{Key: "items_invalid_versions", Label: "适用版本格式非法", Category: "items",
			Severity: SeverityError, Count: n, Total: itemsTotal, Hint: "supported_versions 中存在无法解析或重复的版本号。"})
	}

	// ---- 配方 ----
	recipesTotal, err := s.count(db, &domain.Recipe{}, "", nil)
	if err != nil {
		return nil, err
	}
	out.add(Check{Key: "recipes_total", Label: "配方总数", Category: "recipes",
		Severity: SeverityInfo, Count: recipesTotal, Total: recipesTotal, Hint: "合成/烧炼等配方条目数。"})

	for _, c := range []struct{ key, label, where, hint string }{
		{"recipes_missing_source", "缺少来源 URL", "source_url IS NULL OR source_url = ''", "补齐 source_url 以便溯源。"},
		{"recipes_missing_versions", "缺少适用版本", "supported_versions = '[]'::jsonb", "补齐 supported_versions。"},
	} {
		n, err := s.count(db, &domain.Recipe{}, c.where, nil)
		if err != nil {
			return nil, err
		}
		out.add(Check{Key: c.key, Label: c.label, Category: "recipes",
			Severity: SeverityWarning, Count: n, Total: recipesTotal, Hint: c.hint})
	}
	if n, err := s.invalidVersions(ctx, &domain.Recipe{}, "supported_versions"); err != nil {
		return nil, err
	} else {
		out.add(Check{Key: "recipes_invalid_versions", Label: "适用版本格式非法", Category: "recipes",
			Severity: SeverityError, Count: n, Total: recipesTotal, Hint: "supported_versions 中存在无法解析或重复的版本号。"})
	}
	// 悬空引用：结果物品不在物品库中（数据不一致，会造成详情页断链）。
	var orphan int64
	if err := db.Raw(`SELECT count(*) FROM recipes r
		LEFT JOIN items i ON i.item_id = r.result_item_id
		WHERE i.id IS NULL`).Scan(&orphan).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "recipes_orphan_result", Label: "结果物品不存在于物品库", Category: "recipes",
		Severity: SeverityError, Count: orphan, Total: recipesTotal, Hint: "recipe.result_item_id 未匹配到 items.item_id，会导致详情页断链。"})

	// ---- 指令 ----
	commandsTotal, err := s.count(db, &domain.Command{}, "", nil)
	if err != nil {
		return nil, err
	}
	out.add(Check{Key: "commands_total", Label: "指令总数", Category: "commands",
		Severity: SeverityInfo, Count: commandsTotal, Total: commandsTotal, Hint: "指令资料条目数。"})

	for _, c := range []struct{ key, label, where, hint string }{
		{"commands_missing_source", "缺少来源 URL", "source_url IS NULL OR source_url = ''", "补齐 source_url 以便溯源。"},
		{"commands_missing_versions", "缺少适用版本", "game_versions = '[]'::jsonb", "补齐 game_versions。"},
	} {
		n, err := s.count(db, &domain.Command{}, c.where, nil)
		if err != nil {
			return nil, err
		}
		out.add(Check{Key: c.key, Label: c.label, Category: "commands",
			Severity: SeverityWarning, Count: n, Total: commandsTotal, Hint: c.hint})
	}
	if n, err := s.invalidVersions(ctx, &domain.Command{}, "game_versions"); err != nil {
		return nil, err
	} else {
		out.add(Check{Key: "commands_invalid_versions", Label: "适用版本格式非法", Category: "commands",
			Severity: SeverityError, Count: n, Total: commandsTotal, Hint: "game_versions 中存在无法解析或重复的版本号。"})
	}

	// ---- 药水与酿造（TODO §9A）----
	potionsTotal, err := s.count(db, &domain.Potion{}, "", nil)
	if err != nil {
		return nil, err
	}
	out.add(Check{Key: "potions_total", Label: "药水总数", Category: "potions",
		Severity: SeverityInfo, Count: potionsTotal, Total: potionsTotal, Hint: "药水资料库条目数。"})

	for _, c := range []struct{ key, label, where, hint string }{
		{"potions_missing_source", "缺少来源 URL", "source_url IS NULL OR source_url = ''", "补齐 source_url 以便溯源（TODO §9A）。"},
		{"potions_missing_versions", "缺少适用版本", "supported_versions = '[]'::jsonb", "补齐 supported_versions，避免版本适用性不清。"},
		{"potions_missing_effects", "缺少效果数据", "category IN ('positive','negative','mixed') AND effects = '[]'::jsonb", "补齐 effects，否则详情页无法展示效果与时长。"},
		{"potions_unverified", "超过 365 天未校验", "last_verified_at IS NULL OR last_verified_at < now() - interval '365 days'", "复核药水资料并更新 last_verified_at。"},
	} {
		n, err := s.count(db, &domain.Potion{}, c.where, nil)
		if err != nil {
			return nil, err
		}
		out.add(Check{Key: c.key, Label: c.label, Category: "potions",
			Severity: SeverityWarning, Count: n, Total: potionsTotal, Hint: c.hint})
	}
	if n, err := s.invalidVersions(ctx, &domain.Potion{}, "supported_versions"); err != nil {
		return nil, err
	} else {
		out.add(Check{Key: "potions_invalid_versions", Label: "适用版本格式非法", Category: "potions",
			Severity: SeverityError, Count: n, Total: potionsTotal, Hint: "supported_versions 中存在无法解析或重复的版本号。"})
	}

	brewingTotal, err := s.count(db, &domain.BrewingRecipe{}, "", nil)
	if err != nil {
		return nil, err
	}
	out.add(Check{Key: "brewing_total", Label: "酿造步骤总数", Category: "potions",
		Severity: SeverityInfo, Count: brewingTotal, Total: brewingTotal, Hint: "酿造步骤条目数。"})

	// 悬空引用：输入/结果药水不在药水库中（数据不一致，会造成详情页断链）。
	for _, c := range []struct{ key, label, column, hint string }{
		{"brewing_orphan_result", "结果药水不存在于药水库", "result_potion_slug", "brewing_recipes.result_potion_slug 未匹配到 potions.slug。"},
		{"brewing_orphan_input", "输入药水不存在于药水库", "input_potion_slug", "brewing_recipes.input_potion_slug 未匹配到 potions.slug。"},
	} {
		var n int64
		if err := db.Raw(`SELECT count(*) FROM brewing_recipes b
			LEFT JOIN potions p ON p.slug = b.` + c.column + `
			WHERE b.` + c.column + ` IS NOT NULL AND p.id IS NULL`).Scan(&n).Error; err != nil {
			return nil, err
		}
		out.add(Check{Key: c.key, Label: c.label, Category: "potions",
			Severity: SeverityError, Count: n, Total: brewingTotal, Hint: c.hint})
	}
	// 酿造原料未收录（提醒，非阻断）：不影响酿造资料展示，仅影响跳转到物品详情。
	var missingIngredients int64
	if err := db.Raw(`SELECT count(*) FROM brewing_recipes b
		LEFT JOIN items i ON i.item_id = b.ingredient_item_id
		WHERE i.id IS NULL`).Scan(&missingIngredients).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "brewing_missing_ingredients", Label: "酿造原料未收录于物品库", Category: "potions",
		Severity: SeverityWarning, Count: missingIngredients, Total: brewingTotal,
		Hint: "原料未收录时页面回退为纯文本 ID；如需跳转请在物品库补录。"})
	for _, c := range []struct{ key, label, where, hint string }{
		{"brewing_missing_source", "缺少来源 URL", "source_url IS NULL OR source_url = ''", "补齐 source_url 以便溯源。"},
		{"brewing_missing_versions", "缺少适用版本", "supported_versions = '[]'::jsonb", "补齐 supported_versions。"},
	} {
		n, err := s.count(db, &domain.BrewingRecipe{}, c.where, nil)
		if err != nil {
			return nil, err
		}
		out.add(Check{Key: c.key, Label: c.label, Category: "potions",
			Severity: SeverityWarning, Count: n, Total: brewingTotal, Hint: c.hint})
	}
	if n, err := s.invalidVersions(ctx, &domain.BrewingRecipe{}, "supported_versions"); err != nil {
		return nil, err
	} else {
		out.add(Check{Key: "brewing_invalid_versions", Label: "适用版本格式非法", Category: "potions",
			Severity: SeverityError, Count: n, Total: brewingTotal, Hint: "supported_versions 中存在无法解析或重复的版本号。"})
	}

	// ---- 我的世界开发栏目（TODO §21）----
	devDocsTotal, err := s.count(db, &domain.DevDoc{}, "", nil)
	if err != nil {
		return nil, err
	}
	out.add(Check{Key: "dev_docs_total", Label: "开发文档总数", Category: "dev_docs",
		Severity: SeverityInfo, Count: devDocsTotal, Total: devDocsTotal, Hint: "开发栏目知识库条目数。"})
	for _, c := range []struct{ key, label, where, hint string }{
		{"dev_docs_missing_source", "缺少来源 URL", "source_url IS NULL OR source_url = ''", "补齐 source_url 以便溯源（TODO §21）。"},
		{"dev_docs_missing_versions", "缺少适用版本", "mc_versions = '[]'::jsonb", "补齐 mc_versions，避免版本适用性不清。"},
		{"dev_docs_missing_official", "缺少官方文档链接", "official_url IS NULL OR official_url = ''", "补齐 official_url，保证条目能回到官方文档。"},
		{"dev_docs_missing_api", "未列出关键接口", "api_entries = '[]'::jsonb", "补充 api_entries，提升「接口速查」价值。"},
		{"dev_docs_unverified", "超过 365 天未校验", "last_verified_at IS NULL OR last_verified_at < now() - interval '365 days'", "复核文档条目并更新 last_verified_at。"},
		{"dev_docs_draft", "仍为草稿状态", "status = 'draft'", "草稿不会出现在前台，确认后可改为 published。"},
	} {
		n, err := s.count(db, &domain.DevDoc{}, c.where, nil)
		if err != nil {
			return nil, err
		}
		out.add(Check{Key: c.key, Label: c.label, Category: "dev_docs",
			Severity: SeverityWarning, Count: n, Total: devDocsTotal, Hint: c.hint})
	}
	if n, err := s.invalidVersions(ctx, &domain.DevDoc{}, "mc_versions"); err != nil {
		return nil, err
	} else {
		out.add(Check{Key: "dev_docs_invalid_versions", Label: "适用版本格式非法", Category: "dev_docs",
			Severity: SeverityError, Count: n, Total: devDocsTotal, Hint: "mc_versions 中存在无法解析或重复的版本号。"})
	}

	// 映射数据集：本站不预置数据，故「零数据集」为提醒而非错误。
	mappingSetsTotal, err := s.count(db, &domain.MappingSet{}, "", nil)
	if err != nil {
		return nil, err
	}
	out.add(Check{Key: "mapping_sets_total", Label: "映射数据集总数", Category: "dev_docs",
		Severity: SeverityInfo, Count: mappingSetsTotal, Total: mappingSetsTotal,
		Hint: "由 cmd/import-mappings 导入；为 0 时映射查询页展示导入指引。"})
	for _, c := range []struct{ key, label, where, hint string }{
		{"mapping_sets_missing_source", "缺少来源 URL", "source_url IS NULL OR source_url = ''", "导入时用 --source-url 记录官方下载地址。"},
		{"mapping_sets_empty", "数据集内无类映射", "class_count = 0", "该数据集未包含任何类，建议重新导入或删除。"},
	} {
		n, err := s.count(db, &domain.MappingSet{}, c.where, nil)
		if err != nil {
			return nil, err
		}
		out.add(Check{Key: c.key, Label: c.label, Category: "dev_docs",
			Severity: SeverityWarning, Count: n, Total: mappingSetsTotal, Hint: c.hint})
	}
	// 悬空引用：成员所属类不存在（理论上由外键约束保证，仍做一次防御性检查）。
	var orphanMembers int64
	if err := db.Raw(`SELECT count(*) FROM mapping_members m
		LEFT JOIN mapping_classes c ON c.id = m.class_id WHERE c.id IS NULL`).Scan(&orphanMembers).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "mapping_members_orphan", Label: "成员缺少所属类", Category: "dev_docs",
		Severity: SeverityError, Count: orphanMembers, Total: orphanMembers, Hint: "mapping_members.class_id 未匹配到 mapping_classes。"})

	// ---- 村民交易（TODO §9B）----
	villagerProfTotal, err := s.count(db, &domain.VillagerProfession{}, "", nil)
	if err != nil {
		return nil, err
	}
	out.add(Check{Key: "villager_professions_total", Label: "村民职业总数", Category: "villagers",
		Severity: SeverityInfo, Count: villagerProfTotal, Total: villagerProfTotal, Hint: "村民职业条目数。"})
	for _, c := range []struct{ key, label, where, hint string }{
		{"villager_professions_missing_source", "缺少来源 URL", "source_url IS NULL OR source_url = ''", "补齐 source_url 以便溯源（TODO §9B）。"},
		{"villager_professions_missing_versions", "缺少适用版本", "supported_versions = '[]'::jsonb", "补齐 supported_versions，避免版本适用性不清。"},
		{"villager_professions_unverified", "超过 365 天未校验", "last_verified_at IS NULL OR last_verified_at < now() - interval '365 days'", "复核职业资料并更新 last_verified_at。"},
	} {
		n, err := s.count(db, &domain.VillagerProfession{}, c.where, nil)
		if err != nil {
			return nil, err
		}
		out.add(Check{Key: c.key, Label: c.label, Category: "villagers",
			Severity: SeverityWarning, Count: n, Total: villagerProfTotal, Hint: c.hint})
	}
	if n, err := s.invalidVersions(ctx, &domain.VillagerProfession{}, "supported_versions"); err != nil {
		return nil, err
	} else {
		out.add(Check{Key: "villager_professions_invalid_versions", Label: "适用版本格式非法", Category: "villagers",
			Severity: SeverityError, Count: n, Total: villagerProfTotal, Hint: "supported_versions 中存在无法解析或重复的版本号。"})
	}

	villagerLevelTotal, err := s.count(db, &domain.VillagerTradeLevel{}, "", nil)
	if err != nil {
		return nil, err
	}
	out.add(Check{Key: "villager_trade_levels_total", Label: "职业等级总数", Category: "villagers",
		Severity: SeverityInfo, Count: villagerLevelTotal, Total: villagerLevelTotal,
		Hint: "职业等级定义条数（新手→大师应为 5）。"})

	villagerTradeTotal, err := s.count(db, &domain.VillagerTrade{}, "", nil)
	if err != nil {
		return nil, err
	}
	out.add(Check{Key: "villager_trades_total", Label: "村民交易总数", Category: "villagers",
		Severity: SeverityInfo, Count: villagerTradeTotal, Total: villagerTradeTotal, Hint: "交易表条目数。"})
	for _, c := range []struct{ key, label, where, hint string }{
		{"villager_trades_missing_source", "缺少来源 URL", "source_url IS NULL OR source_url = ''", "补齐 source_url 以便溯源。"},
		{"villager_trades_missing_versions", "缺少适用版本", "supported_versions = '[]'::jsonb", "补齐 supported_versions。"},
	} {
		n, err := s.count(db, &domain.VillagerTrade{}, c.where, nil)
		if err != nil {
			return nil, err
		}
		out.add(Check{Key: c.key, Label: c.label, Category: "villagers",
			Severity: SeverityWarning, Count: n, Total: villagerTradeTotal, Hint: c.hint})
	}
	if n, err := s.invalidVersions(ctx, &domain.VillagerTrade{}, "supported_versions"); err != nil {
		return nil, err
	} else {
		out.add(Check{Key: "villager_trades_invalid_versions", Label: "适用版本格式非法", Category: "villagers",
			Severity: SeverityError, Count: n, Total: villagerTradeTotal, Hint: "supported_versions 中存在无法解析或重复的版本号。"})
	}
	// 悬空引用：交易所属职业不存在（数据不一致，会造成详情页断链）。
	var orphanTradeProfession int64
	if err := db.Raw(`SELECT count(*) FROM villager_trades t
		LEFT JOIN villager_professions p ON p.slug = t.profession_slug
		WHERE p.id IS NULL`).Scan(&orphanTradeProfession).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "villager_trades_orphan_profession", Label: "交易所属职业不存在", Category: "villagers",
		Severity: SeverityError, Count: orphanTradeProfession, Total: villagerTradeTotal,
		Hint: "villager_trades.profession_slug 未匹配到 villager_professions.slug。"})
	// 等级与分组的约束（与导入器校验一致，防御历史数据）。
	var badLevel int64
	if err := db.Raw(`SELECT count(*) FROM villager_trades
		WHERE (trade_group = 'level' AND (level < 1 OR level > 5))
		   OR (trade_group <> 'level' AND level <> 0)`).Scan(&badLevel).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "villager_trades_bad_level", Label: "等级与交易分组不匹配", Category: "villagers",
		Severity: SeverityError, Count: badLevel, Total: villagerTradeTotal,
		Hint: "trade_group=level 的交易 level 必须为 1–5，其余分组必须为 0。"})
	// 结果物品未收录（提醒，非阻断）：仅影响跳转到物品详情。
	var missingResultItems int64
	if err := db.Raw(`SELECT count(*) FROM villager_trades t
		LEFT JOIN items i ON i.item_id = t.result_item_id
		WHERE i.id IS NULL`).Scan(&missingResultItems).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "villager_trades_missing_result_items", Label: "结果物品未收录于物品库", Category: "villagers",
		Severity: SeverityWarning, Count: missingResultItems, Total: villagerTradeTotal,
		Hint: "结果物品未收录时页面回退为纯文本 ID；如需跳转请在物品库补录。"})
	// 成本物品未收录（提醒）：JSONB 数组逐项匹配物品库。
	var missingCostItems int64
	if err := db.Raw(`SELECT count(*) FROM villager_trades t,
		jsonb_array_elements(t.cost_items) c
		LEFT JOIN items i ON i.item_id = c->>'item_id'
		WHERE i.id IS NULL`).Scan(&missingCostItems).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "villager_trades_missing_cost_items", Label: "成本物品未收录于物品库", Category: "villagers",
		Severity: SeverityWarning, Count: missingCostItems, Total: villagerTradeTotal,
		Hint: "成本物品未收录时页面回退为纯文本 ID；如需跳转请在物品库补录。"})
	// 可交易职业无任何交易（数据缺口）。
	var normalWithoutTrades int64
	if err := db.Raw(`SELECT count(*) FROM villager_professions p
		WHERE p.kind = 'normal'
		  AND NOT EXISTS (SELECT 1 FROM villager_trades t WHERE t.profession_slug = p.slug)`).Scan(&normalWithoutTrades).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "villager_professions_without_trades", Label: "可交易职业缺少交易表", Category: "villagers",
		Severity: SeverityWarning, Count: normalWithoutTrades, Total: villagerProfTotal,
		Hint: "kind=normal 的职业应至少有一条交易。"})

	// ---- 生物资料（TODO §9C）----
	mobTotal, err := s.count(db, &domain.Mob{}, "", nil)
	if err != nil {
		return nil, err
	}
	out.add(Check{Key: "mobs_total", Label: "生物总数", Category: "mobs",
		Severity: SeverityInfo, Count: mobTotal, Total: mobTotal, Hint: "生物资料条目数。"})

	for _, c := range []struct{ key, label, where, hint string }{
		{"mobs_missing_source", "缺少来源 URL", "source_url IS NULL OR source_url = ''", "补齐 source_url 以便溯源（TODO §9C）。"},
		{"mobs_missing_versions", "缺少适用版本", "supported_versions = '[]'::jsonb", "补齐 supported_versions，避免版本适用性不清。"},
		{"mobs_unverified", "超过 365 天未校验", "last_verified_at IS NULL OR last_verified_at < now() - interval '365 days'", "复核生物资料并更新 last_verified_at。"},
	} {
		n, err := s.count(db, &domain.Mob{}, c.where, nil)
		if err != nil {
			return nil, err
		}
		out.add(Check{Key: c.key, Label: c.label, Category: "mobs",
			Severity: SeverityWarning, Count: n, Total: mobTotal, Hint: c.hint})
	}
	if n, err := s.invalidVersions(ctx, &domain.Mob{}, "supported_versions"); err != nil {
		return nil, err
	} else if n > 0 {
		out.add(Check{Key: "mobs_invalid_versions", Label: "适用版本格式非法", Category: "mobs",
			Severity: SeverityError, Count: n, Total: mobTotal, Hint: "supported_versions 中存在无法解析或重复的版本号。"})
	}

	mobDropTotal, err := s.count(db, &domain.MobDrop{}, "", nil)
	if err != nil {
		return nil, err
	}
	out.add(Check{Key: "mob_drops_total", Label: "生物掉落总数", Category: "mobs",
		Severity: SeverityInfo, Count: mobDropTotal, Total: mobDropTotal, Hint: "掉落表条目数。"})

	// 掉落所属生物不存在（悬空引用）。
	var orphanDropMob int64
	if err := db.Raw(`SELECT count(*) FROM mob_drops d
		LEFT JOIN mobs m ON m.slug = d.mob_slug
		WHERE m.id IS NULL`).Scan(&orphanDropMob).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "mob_drops_orphan_mob", Label: "掉落所属生物不存在", Category: "mobs",
		Severity: SeverityError, Count: orphanDropMob, Total: mobDropTotal,
		Hint: "mob_drops.mob_slug 未匹配到 mobs.slug。"})

	// 数量区间非法。
	var badDropCount int64
	if err := db.Raw(`SELECT count(*) FROM mob_drops
		WHERE count_min < 0 OR count_max < count_min`).Scan(&badDropCount).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "mob_drops_bad_count", Label: "掉落数量区间非法", Category: "mobs",
		Severity: SeverityError, Count: badDropCount, Total: mobDropTotal,
		Hint: "count_min 不能为负，且 count_max 不能小于 count_min。"})

	// 概率越界。
	var badDropChance int64
	if err := db.Raw(`SELECT count(*) FROM mob_drops
		WHERE chance < 0 OR chance > 1`).Scan(&badDropChance).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "mob_drops_bad_chance", Label: "掉落概率越界", Category: "mobs",
		Severity: SeverityError, Count: badDropChance, Total: mobDropTotal,
		Hint: "chance 必须在 0–1 之间（1 表示必定掉落）。"})

	// 掉落物未收录于物品库（仅提示，不阻断；前端回退为纯文本 ID）。
	var missingDropItems int64
	if err := db.Raw(`SELECT count(*) FROM mob_drops d
		LEFT JOIN items i ON i.item_id = d.item_id
		WHERE i.id IS NULL`).Scan(&missingDropItems).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "mob_drops_missing_items", Label: "掉落物未收录于物品库", Category: "mobs",
		Severity: SeverityWarning, Count: missingDropItems, Total: mobDropTotal,
		Hint: "掉落物未收录时页面回退为纯文本 ID；如需跳转请在物品库补录。"})

	// ---- 红石与机器设计（TODO §15）----
	redstoneComponentTotal, err := s.count(db, &domain.RedstoneComponent{}, "", nil)
	if err != nil {
		return nil, err
	}
	out.add(Check{Key: "redstone_components_total", Label: "红石元件总数", Category: "redstone",
		Severity: SeverityInfo, Count: redstoneComponentTotal, Total: redstoneComponentTotal,
		Hint: "红石元件资料条目数。"})
	for _, c := range []struct{ key, label, where, hint string }{
		{"redstone_components_missing_source", "缺少来源 URL", "source_url IS NULL OR source_url = ''", "补齐 source_url 以便溯源（TODO §15）。"},
		{"redstone_components_missing_versions", "缺少适用版本", "supported_versions = '[]'::jsonb", "补齐 supported_versions，避免版本适用性不清。"},
		{"redstone_components_unverified", "超过 365 天未校验", "last_verified_at IS NULL OR last_verified_at < now() - interval '365 days'", "复核元件资料并更新 last_verified_at。"},
	} {
		n, err := s.count(db, &domain.RedstoneComponent{}, c.where, nil)
		if err != nil {
			return nil, err
		}
		out.add(Check{Key: c.key, Label: c.label, Category: "redstone",
			Severity: SeverityWarning, Count: n, Total: redstoneComponentTotal, Hint: c.hint})
	}
	if n, err := s.invalidVersions(ctx, &domain.RedstoneComponent{}, "supported_versions"); err != nil {
		return nil, err
	} else {
		out.add(Check{Key: "redstone_components_invalid_versions", Label: "适用版本格式非法", Category: "redstone",
			Severity: SeverityError, Count: n, Total: redstoneComponentTotal, Hint: "supported_versions 中存在无法解析或重复的版本号。"})
	}

	redstoneDesignTotal, err := s.count(db, &domain.RedstoneDesign{}, "", nil)
	if err != nil {
		return nil, err
	}
	out.add(Check{Key: "redstone_designs_total", Label: "机器设计总数", Category: "redstone",
		Severity: SeverityInfo, Count: redstoneDesignTotal, Total: redstoneDesignTotal,
		Hint: "机器设计条目数（含待审核与已驳回）。"})
	for _, c := range []struct{ key, label, where, hint string }{
		{"redstone_designs_missing_source", "缺少来源 URL", "source_url IS NULL OR source_url = ''", "补齐 source_url 以便溯源（TODO §15）。"},
		{"redstone_designs_missing_versions", "缺少适用版本", "supported_versions = '[]'::jsonb", "补齐 supported_versions，避免版本适用性不清。"},
		{"redstone_designs_unverified", "超过 365 天未校验", "last_verified_at IS NULL OR last_verified_at < now() - interval '365 days'", "复核设计资料并更新 last_verified_at。"},
		{"redstone_designs_unpublished", "未发布（待审核 / 已驳回）", "review_status <> 'published'", "未发布设计不对公开接口可见；审核通过后改为 published。"},
	} {
		n, err := s.count(db, &domain.RedstoneDesign{}, c.where, nil)
		if err != nil {
			return nil, err
		}
		out.add(Check{Key: c.key, Label: c.label, Category: "redstone",
			Severity: SeverityWarning, Count: n, Total: redstoneDesignTotal, Hint: c.hint})
	}
	if n, err := s.invalidVersions(ctx, &domain.RedstoneDesign{}, "supported_versions"); err != nil {
		return nil, err
	} else {
		out.add(Check{Key: "redstone_designs_invalid_versions", Label: "适用版本格式非法", Category: "redstone",
			Severity: SeverityError, Count: n, Total: redstoneDesignTotal, Hint: "supported_versions 中存在无法解析或重复的版本号。"})
	}
	// 材料引用未收录物品（提醒，非阻断）：item_id 非空但未匹配到物品库，页面回退为纯文本。
	var danglingMaterials int64
	if err := db.Raw(`SELECT count(*) FROM redstone_designs d
		WHERE EXISTS (
			SELECT 1 FROM jsonb_array_elements(d.materials) m
			LEFT JOIN items i ON i.item_id = m->>'item_id'
			WHERE COALESCE(m->>'item_id','') <> '' AND i.id IS NULL
		)`).Scan(&danglingMaterials).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "redstone_designs_dangling_materials", Label: "材料引用未收录物品", Category: "redstone",
		Severity: SeverityWarning, Count: danglingMaterials, Total: redstoneDesignTotal,
		Hint: "materials 中 item_id 非空但未匹配到物品库；未收录时页面回退为纯文本，如需跳转请在物品库补录。"})
	// 材料数量区间非法（数据不一致）。
	var badMaterialCount int64
	if err := db.Raw(`SELECT count(*) FROM redstone_designs d
		WHERE EXISTS (
			SELECT 1 FROM jsonb_array_elements(d.materials) m
			WHERE (m->>'count_min')::int < 1
			   OR (m->>'count_max')::int < (m->>'count_min')::int
		)`).Scan(&badMaterialCount).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "redstone_designs_bad_material_count", Label: "材料数量区间非法", Category: "redstone",
		Severity: SeverityError, Count: badMaterialCount, Total: redstoneDesignTotal,
		Hint: "materials 每项 count_min 必须 ≥ 1，且 count_max 不能小于 count_min。"})
	// 未授权却带存档下载链接（版权红线）。
	var unauthorizedDownload int64
	if err := db.Raw(`SELECT count(*) FROM redstone_designs
		WHERE world_download_url <> '' AND (license IS NULL OR license = '')`).Scan(&unauthorizedDownload).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "redstone_designs_unauthorized_download", Label: "未授权却带存档下载链接", Category: "redstone",
		Severity: SeverityError, Count: unauthorizedDownload, Total: redstoneDesignTotal,
		Hint: "world_download_url 非空时必须提供 license；未授权存档一律不得提供下载链接（TODO §15）。"})

	// ---- Mod 与整合包（TODO §16）----
	modTotal, err := s.count(db, &domain.Mod{}, "", nil)
	if err != nil {
		return nil, err
	}
	out.add(Check{Key: "mods_total", Label: "Mod 总数", Category: "mods",
		Severity: SeverityInfo, Count: modTotal, Total: modTotal,
		Hint: "Mod 条目数（含待审核与已驳回）。"})
	for _, c := range []struct{ key, label, where, hint string }{
		{"mods_missing_source", "缺少来源 URL", "source_url IS NULL OR source_url = ''", "补齐 source_url 作为可核验锚点（TODO §16）。"},
		{"mods_missing_versions", "缺少适用版本", "mc_versions = '[]'::jsonb", "补齐 mc_versions，避免版本适用性不清。"},
		{"mods_unverified", "超过 365 天未校验", "last_verified_at IS NULL OR last_verified_at < now() - interval '365 days'", "复核 Mod 资料并更新 last_verified_at。"},
		{"mods_unpublished", "未发布（待审核 / 已驳回）", "review_status <> 'published'", "未发布 Mod 不对公开接口可见；审核通过后改为 published。"},
	} {
		n, err := s.count(db, &domain.Mod{}, c.where, nil)
		if err != nil {
			return nil, err
		}
		out.add(Check{Key: c.key, Label: c.label, Category: "mods",
			Severity: SeverityWarning, Count: n, Total: modTotal, Hint: c.hint})
	}
	if n, err := s.invalidVersions(ctx, &domain.Mod{}, "mc_versions"); err != nil {
		return nil, err
	} else {
		out.add(Check{Key: "mods_invalid_versions", Label: "适用版本格式非法", Category: "mods",
			Severity: SeverityError, Count: n, Total: modTotal, Hint: "mc_versions 中存在无法解析或重复的版本号。"})
	}

	// ---- Mod 发布版本 ----
	modVersionTotal, err := s.count(db, &domain.ModVersion{}, "", nil)
	if err != nil {
		return nil, err
	}
	out.add(Check{Key: "mod_versions_total", Label: "Mod 发布版本总数", Category: "mods",
		Severity: SeverityInfo, Count: modVersionTotal, Total: modVersionTotal,
		Hint: "同一 Mod 的不同发布版本条目数。"})
	for _, c := range []struct{ key, label, where, hint string }{
		{"mod_versions_missing_versions", "缺少适用版本", "mc_versions = '[]'::jsonb", "补齐版本条目自身的 mc_versions。"},
		{"mod_versions_unverified", "未标注验证状态", "verified = false", "核验下载入口后置 verified = true，并补 verify_note。"},
	} {
		n, err := s.count(db, &domain.ModVersion{}, c.where, nil)
		if err != nil {
			return nil, err
		}
		out.add(Check{Key: c.key, Label: c.label, Category: "mods",
			Severity: SeverityWarning, Count: n, Total: modVersionTotal, Hint: c.hint})
	}
	// 非法发布版本类型（DB CHECK 已约束，此处防御性统计）。
	var badReleaseType int64
	if err := db.Raw(`SELECT count(*) FROM mod_versions
		WHERE release_type NOT IN ('release', 'beta', 'alpha')`).Scan(&badReleaseType).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "mod_versions_invalid_release_type", Label: "发布版本类型非法", Category: "mods",
		Severity: SeverityError, Count: badReleaseType, Total: modVersionTotal,
		Hint: "release_type 必须属于 release / beta / alpha。"})
	// 重复版本（唯一键已约束，此处防御性统计）。
	var dupVersions int64
	if err := db.Raw(`SELECT COALESCE(sum(c), 0) FROM (
		SELECT count(*) - 1 AS c FROM mod_versions GROUP BY mod_id, version HAVING count(*) > 1
	) t`).Scan(&dupVersions).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "mod_versions_duplicate", Label: "同一 Mod 重复版本", Category: "mods",
		Severity: SeverityError, Count: dupVersions, Total: modVersionTotal,
		Hint: "同一 Mod 的 (mod_id, version) 不得重复。"})
	if n, err := s.invalidVersions(ctx, &domain.ModVersion{}, "mc_versions"); err != nil {
		return nil, err
	} else {
		out.add(Check{Key: "mod_versions_invalid_versions", Label: "适用版本格式非法", Category: "mods",
			Severity: SeverityError, Count: n, Total: modVersionTotal, Hint: "版本条目的 mc_versions 中存在无法解析或重复的版本号。"})
	}
	// 下载链接缺授权说明（版权红线）：download_url 非空但父 Mod 无 license / license_note。
	var unauthorizedModDownload int64
	if err := db.Raw(`SELECT count(*) FROM mod_versions v
		JOIN mods m ON m.id = v.mod_id
		WHERE v.download_url <> ''
		  AND COALESCE(m.license, '') = '' AND COALESCE(m.license_note, '') = ''`).Scan(&unauthorizedModDownload).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "mod_versions_unauthorized_download", Label: "下载链接缺授权说明", Category: "mods",
		Severity: SeverityError, Count: unauthorizedModDownload, Total: modVersionTotal,
		Hint: "download_url 非空时所属 Mod 的 license 或 license_note 必须说明来源授权（不未经许可镜像分发）。"})

	// ---- Mod 依赖与冲突 ----
	modDepTotal, err := s.count(db, &domain.ModDependency{}, "", nil)
	if err != nil {
		return nil, err
	}
	out.add(Check{Key: "mod_dependencies_total", Label: "Mod 依赖 / 冲突总数", Category: "mods",
		Severity: SeverityInfo, Count: modDepTotal, Total: modDepTotal,
		Hint: "已记录的依赖与冲突关系条目数。"})
	// 悬空 mod 引用：target_mod_id 非空但目标已不存在。
	var danglingDeps int64
	if err := db.Raw(`SELECT count(*) FROM mod_dependencies d
		WHERE d.target_mod_id IS NOT NULL
		  AND NOT EXISTS (SELECT 1 FROM mods m WHERE m.id = d.target_mod_id)`).Scan(&danglingDeps).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "mod_dependencies_dangling", Label: "依赖引用已删除的 Mod", Category: "mods",
		Severity: SeverityError, Count: danglingDeps, Total: modDepTotal,
		Hint: "target_mod_id 指向的 Mod 已删除；应改为 target_mod_name 纯文本。"})
	var badDepKind int64
	if err := db.Raw(`SELECT count(*) FROM mod_dependencies
		WHERE dep_kind NOT IN ('required', 'optional', 'embedded', 'incompatible')`).Scan(&badDepKind).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "mod_dependencies_invalid_kind", Label: "依赖类型非法", Category: "mods",
		Severity: SeverityError, Count: badDepKind, Total: modDepTotal,
		Hint: "dep_kind 必须属于 required / optional / embedded / incompatible。"})
	var incompatibleNoNote int64
	if err := db.Raw(`SELECT count(*) FROM mod_dependencies
		WHERE dep_kind = 'incompatible' AND COALESCE(note, '') = ''`).Scan(&incompatibleNoNote).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "mod_dependencies_incompatible_missing_note", Label: "不兼容关系缺少说明", Category: "mods",
		Severity: SeverityError, Count: incompatibleNoNote, Total: modDepTotal,
		Hint: "dep_kind = incompatible 时必须给出 note（冲突依据）。"})
	var missingTarget int64
	if err := db.Raw(`SELECT count(*) FROM mod_dependencies
		WHERE target_mod_id IS NULL AND COALESCE(target_mod_name, '') = ''`).Scan(&missingTarget).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "mod_dependencies_missing_target", Label: "依赖缺少目标", Category: "mods",
		Severity: SeverityError, Count: missingTarget, Total: modDepTotal,
		Hint: "target_mod_id 与 target_mod_name 必须至少提供其一。"})

	// ---- 整合包 ----
	modpackTotal, err := s.count(db, &domain.Modpack{}, "", nil)
	if err != nil {
		return nil, err
	}
	out.add(Check{Key: "modpacks_total", Label: "整合包总数", Category: "mods",
		Severity: SeverityInfo, Count: modpackTotal, Total: modpackTotal,
		Hint: "整合包条目数（含待审核与已驳回）。"})
	for _, c := range []struct{ key, label, where, hint string }{
		{"modpacks_missing_source", "缺少来源 URL", "source_url IS NULL OR source_url = ''", "补齐 source_url 作为可核验锚点（TODO §16）。"},
		{"modpacks_unverified", "超过 365 天未校验", "last_verified_at IS NULL OR last_verified_at < now() - interval '365 days'", "复核整合包资料并更新 last_verified_at。"},
		{"modpacks_unpublished", "未发布（待审核 / 已驳回）", "review_status <> 'published'", "未发布整合包不对公开接口可见；审核通过后改为 published。"},
	} {
		n, err := s.count(db, &domain.Modpack{}, c.where, nil)
		if err != nil {
			return nil, err
		}
		out.add(Check{Key: c.key, Label: c.label, Category: "mods",
			Severity: SeverityWarning, Count: n, Total: modpackTotal, Hint: c.hint})
	}
	var badPackLoader int64
	if err := db.Raw(`SELECT count(*) FROM modpacks
		WHERE COALESCE(loader, '') <> '' AND loader NOT IN ('fabric', 'forge', 'neoforge', 'quilt')`).Scan(&badPackLoader).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "modpacks_invalid_loader", Label: "加载器非法", Category: "mods",
		Severity: SeverityError, Count: badPackLoader, Total: modpackTotal,
		Hint: "loader 必须属于 fabric / forge / neoforge / quilt（或留空）。"})

	// ---- 整合包内 Mod 清单 ----
	modpackModTotal, err := s.count(db, &domain.ModpackMod{}, "", nil)
	if err != nil {
		return nil, err
	}
	out.add(Check{Key: "modpack_mods_total", Label: "整合包 Mod 清单总数", Category: "mods",
		Severity: SeverityInfo, Count: modpackModTotal, Total: modpackModTotal,
		Hint: "整合包内 Mod 清单条目数。"})
	var danglingPackMods int64
	if err := db.Raw(`SELECT count(*) FROM modpack_mods pm
		WHERE pm.mod_id IS NOT NULL
		  AND NOT EXISTS (SELECT 1 FROM mods m WHERE m.id = pm.mod_id)`).Scan(&danglingPackMods).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "modpack_mods_dangling", Label: "清单引用已删除的 Mod", Category: "mods",
		Severity: SeverityError, Count: danglingPackMods, Total: modpackModTotal,
		Hint: "mod_id 指向的 Mod 已删除；应改为 mod_name 纯文本。"})
	var missingPackModName int64
	if err := db.Raw(`SELECT count(*) FROM modpack_mods
		WHERE mod_id IS NULL AND COALESCE(mod_name, '') = ''`).Scan(&missingPackModName).Error; err != nil {
		return nil, err
	}
	out.add(Check{Key: "modpack_mods_missing_name", Label: "清单项缺少名称", Category: "mods",
		Severity: SeverityError, Count: missingPackModName, Total: modpackModTotal,
		Hint: "mod_id 与 mod_name 必须至少提供其一。"})

	// ---- 导入任务 ----
	recentFailed, err := s.count(db, &domain.DataImportJob{},
		"failed_count > 0 AND created_at > now() - interval '30 days'", nil)
	if err != nil {
		return nil, err
	}
	out.add(Check{Key: "import_recent_failed", Label: "近 30 天存在失败的导入任务", Category: "import",
		Severity: SeverityError, Count: recentFailed, Total: 0, Hint: "查看导入任务列表中的错误摘要并修正数据源。"})

	return out, nil
}

// count 统计表内满足条件的行数；where 为空时统计全表。
func (s *Service) count(db *gorm.DB, model any, where string, args []any) (int64, error) {
	tx := db.Model(model)
	if strings.TrimSpace(where) != "" {
		tx = tx.Where(where, args...)
	}
	var n int64
	err := tx.Count(&n).Error
	return n, err
}

// versionRow 用于扫描「版本列表」列。
type versionRow struct {
	ID       string          `gorm:"column:id"`
	Versions types.JSONSlice `gorm:"column:versions"`
}

// invalidVersions 统计版本列表格式非法 / 重复的行数。
// 版本集合通常很小，直接全量扫描在资料库量级下可接受。
func (s *Service) invalidVersions(ctx context.Context, model any, column string) (int64, error) {
	var rows []versionRow
	if err := s.db.WithContext(ctx).Model(model).
		Select("id", column+" AS versions").
		Scan(&rows).Error; err != nil {
		return 0, err
	}
	var bad int64
	for _, r := range rows {
		if len(r.Versions) == 0 {
			continue
		}
		if errs := versioning.Validate(r.Versions); len(errs) > 0 {
			bad++
		}
	}
	return bad, nil
}

// ListJobs 分页查询导入任务，可按数据类型过滤。
func (s *Service) ListJobs(ctx context.Context, dataType string, page, pageSize int) ([]domain.DataImportJob, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	tx := s.db.WithContext(ctx).Model(&domain.DataImportJob{})
	if dt := strings.TrimSpace(dataType); dt != "" {
		tx = tx.Where("data_type = ?", dt)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	jobs := make([]domain.DataImportJob, 0, pageSize)
	if err := tx.Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&jobs).Error; err != nil {
		return nil, 0, err
	}
	return jobs, total, nil
}
