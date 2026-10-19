// 药水与酿造资料导入（TODO §9A）。
//
// 数据文件为单个 JSON 对象：{ "potions": [...], "brewing_recipes": [...] }。
// 校验策略（对应 §9「校验资料来源和版本适用范围」）：
//   - slug / name_zh / category / supported_versions / source_url 为必填；
//   - 效果条目需 variant 合法、effect_id 非空、amplifier ≥ 1、时长 ≥ 0；
//   - 酿造步骤的 result_potion_slug / input_potion_slug 必须能在同批数据或库中解析，
//     否则视为悬空引用（致命），避免详情页断链；
//   - 缺失 source_url 的酿造步骤回退为「酿造」总览页（该页是全部步骤的唯一来源）。
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

// brewingOverviewSource 全部酿造步骤的默认来源页。
const brewingOverviewSource = "https://minecraft.wiki/w/Brewing"

// potionSeedFile 药水资料文件结构。
type potionSeedFile struct {
	Potions        []potionRow  `json:"potions"`
	BrewingRecipes []brewingRow `json:"brewing_recipes"`
}

// potionRow 药水导入原始行。
type potionRow struct {
	Slug              string                `json:"slug"`
	NameZh            string                `json:"name_zh"`
	NameEn            string                `json:"name_en"`
	Aliases           []string              `json:"aliases"`
	Category          string                `json:"category"`
	Summary           string                `json:"summary"`
	EffectNote        string                `json:"effect_note"`
	IsBrewable        *bool                 `json:"is_brewable"`
	IsExtendable      *bool                 `json:"is_extendable"`
	IsEnhanceable     *bool                 `json:"is_enhanceable"`
	Effects           []domain.PotionEffect `json:"effects"`
	GameEdition       string                `json:"game_edition"`
	SupportedVersions []string              `json:"supported_versions"`
	SourceURL         string                `json:"source_url"`
	DataVersion       string                `json:"data_version"`
	LastVerifiedAt    string                `json:"last_verified_at"`
}

// brewingRow 酿造步骤导入原始行。
type brewingRow struct {
	RecipeKey         string   `json:"recipe_key"`
	StepKind          string   `json:"step_kind"`
	InputPotionSlug   *string  `json:"input_potion_slug"`
	IngredientItemID  string   `json:"ingredient_item_id"`
	IngredientNameZh  string   `json:"ingredient_name_zh"`
	ResultPotionSlug  *string  `json:"result_potion_slug"`
	Note              string   `json:"note"`
	GameEdition       string   `json:"game_edition"`
	SupportedVersions []string `json:"supported_versions"`
	SourceURL         string   `json:"source_url"`
	SortOrder         int      `json:"sort_order"`
}

// validPotionVariants 药水效果变体白名单。
var validPotionVariants = map[string]bool{
	domain.PotionVariantNormal:   true,
	domain.PotionVariantExtended: true,
	domain.PotionVariantEnhanced: true,
}

// validEditions 版本归属白名单。
var validEditions = map[string]bool{"java": true, "bedrock": true, "both": true}

// buildPotion 校验并归一化单行药水资料。
func buildPotion(row potionRow) (*domain.Potion, error) {
	slug := strings.TrimSpace(row.Slug)
	nameZh := strings.TrimSpace(row.NameZh)
	if slug == "" || nameZh == "" {
		return nil, errors.New("slug 与 name_zh 必填")
	}
	category := strings.TrimSpace(row.Category)
	if category == "" {
		category = domain.PotionCategoryOther
	}
	if !domain.IsValidPotionCategory(category) {
		return nil, fmt.Errorf("category 非法: %q", row.Category)
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
		edition = "both"
	}
	if !validEditions[edition] {
		return nil, fmt.Errorf("game_edition 非法: %q", row.GameEdition)
	}

	effects := make(domain.PotionEffects, 0, len(row.Effects))
	for i, e := range row.Effects {
		effect, err := normalizeEffect(i, e)
		if err != nil {
			return nil, err
		}
		effects = append(effects, effect)
	}

	potion := &domain.Potion{
		Slug:              slug,
		NameZh:            nameZh,
		NameEn:            strings.TrimSpace(row.NameEn),
		Aliases:           types.JSONSlice(aliasOrEmpty(row.Aliases)),
		Category:          category,
		Summary:           strings.TrimSpace(row.Summary),
		EffectNote:        strings.TrimSpace(row.EffectNote),
		IsBrewable:        boolOr(row.IsBrewable, true),
		IsExtendable:      boolOr(row.IsExtendable, false),
		IsEnhanceable:     boolOr(row.IsEnhanceable, false),
		Effects:           effects,
		GameEdition:       edition,
		SupportedVersions: types.JSONSlice(versioning.Sort(versions)),
		SourceURL:         sourceURL,
		DataVersion:       strings.TrimSpace(row.DataVersion),
	}
	if raw := strings.TrimSpace(row.LastVerifiedAt); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, fmt.Errorf("last_verified_at 需为 RFC3339 时间: %v", err)
		}
		potion.LastVerifiedAt = &t
	}
	return potion, nil
}

// normalizeEffect 校验并归一化单条效果。
func normalizeEffect(index int, e domain.PotionEffect) (domain.PotionEffect, error) {
	label := fmt.Sprintf("effects[%d]", index)
	variant := strings.TrimSpace(e.Variant)
	if variant == "" {
		variant = domain.PotionVariantNormal
	}
	if !validPotionVariants[variant] {
		return e, fmt.Errorf("%s.variant 非法: %q", label, e.Variant)
	}
	if strings.TrimSpace(e.EffectID) == "" {
		return e, fmt.Errorf("%s.effect_id 必填", label)
	}
	if strings.TrimSpace(e.NameZh) == "" {
		return e, fmt.Errorf("%s.name_zh 必填", label)
	}
	if e.Amplifier < 1 {
		return e, fmt.Errorf("%s.amplifier 至少为 1", label)
	}
	if e.DurationSeconds < 0 {
		return e, fmt.Errorf("%s.duration_seconds 不能为负", label)
	}
	return domain.PotionEffect{
		Variant:         variant,
		EffectID:        strings.TrimSpace(e.EffectID),
		NameZh:          strings.TrimSpace(e.NameZh),
		NameEn:          strings.TrimSpace(e.NameEn),
		Amplifier:       e.Amplifier,
		DurationSeconds: e.DurationSeconds,
		Description:     strings.TrimSpace(e.Description),
	}, nil
}

// buildBrewingRecipe 校验并归一化单条酿造步骤。
// knownSlugs 为同批数据中的药水 slug 集合，用于检出悬空引用。
func buildBrewingRecipe(row brewingRow, knownSlugs map[string]bool) (*domain.BrewingRecipe, error) {
	key := strings.TrimSpace(row.RecipeKey)
	if key == "" {
		return nil, errors.New("recipe_key 必填")
	}
	kind := strings.TrimSpace(row.StepKind)
	if !domain.IsValidBrewStepKind(kind) {
		return nil, fmt.Errorf("step_kind 非法: %q", row.StepKind)
	}
	ingredient := strings.TrimSpace(row.IngredientItemID)
	if ingredient == "" {
		return nil, errors.New("ingredient_item_id 必填")
	}
	if strings.TrimSpace(row.IngredientNameZh) == "" {
		return nil, errors.New("ingredient_name_zh 必填")
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
		edition = "both"
	}
	if !validEditions[edition] {
		return nil, fmt.Errorf("game_edition 非法: %q", row.GameEdition)
	}

	// 悬空引用检查：输入/结果药水必须能在同批数据中解析。
	if row.InputPotionSlug != nil {
		in := strings.TrimSpace(*row.InputPotionSlug)
		if in == "" {
			row.InputPotionSlug = nil
		} else if !knownSlugs[in] {
			return nil, fmt.Errorf("input_potion_slug 未收录: %q", in)
		} else {
			row.InputPotionSlug = &in
		}
	}
	if row.ResultPotionSlug != nil {
		out := strings.TrimSpace(*row.ResultPotionSlug)
		if out == "" {
			row.ResultPotionSlug = nil
		} else if !knownSlugs[out] {
			return nil, fmt.Errorf("result_potion_slug 未收录: %q", out)
		} else {
			row.ResultPotionSlug = &out
		}
	}
	// 基底/效果/腐化步骤必须有结果药水；通用修饰与形态规则必须无结果药水。
	switch kind {
	case domain.BrewStepBase, domain.BrewStepEffect, domain.BrewStepCorrupt:
		if row.ResultPotionSlug == nil {
			return nil, fmt.Errorf("step_kind=%s 必须指定 result_potion_slug", kind)
		}
	case domain.BrewStepModifier, domain.BrewStepForm:
		if row.ResultPotionSlug != nil {
			return nil, fmt.Errorf("step_kind=%s 不应指定 result_potion_slug", kind)
		}
	}

	sourceURL := strings.TrimSpace(row.SourceURL)
	if sourceURL == "" {
		sourceURL = brewingOverviewSource
	}

	return &domain.BrewingRecipe{
		RecipeKey:         key,
		StepKind:          kind,
		InputPotionSlug:   row.InputPotionSlug,
		IngredientItemID:  ingredient,
		IngredientNameZh:  strings.TrimSpace(row.IngredientNameZh),
		ResultPotionSlug:  row.ResultPotionSlug,
		Note:              strings.TrimSpace(row.Note),
		GameEdition:       edition,
		SupportedVersions: types.JSONSlice(versioning.Sort(versions)),
		SourceURL:         sourceURL,
		SortOrder:         row.SortOrder,
	}, nil
}

// importPotions 导入药水与酿造步骤（先药水后步骤，便于引用校验）。
func importPotions(gormDB *gorm.DB, content []byte, dryRun bool) *summary {
	var file potionSeedFile
	if err := json.Unmarshal(content, &file); err != nil {
		fmt.Fprintf(os.Stderr, "JSON 解析失败: %v\n", err)
		os.Exit(1)
	}

	s := &summary{total: len(file.Potions) + len(file.BrewingRecipes)}
	knownSlugs := make(map[string]bool, len(file.Potions))

	// ---- 药水 ----
	for i, row := range file.Potions {
		potion, err := buildPotion(row)
		if err != nil {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("药水第 %d 行：%v", i+1, err))
			continue
		}
		knownSlugs[potion.Slug] = true
		if len(potion.Effects) == 0 && potion.Category != domain.PotionCategoryWater &&
			potion.Category != domain.PotionCategoryBase && potion.Category != domain.PotionCategoryOther {
			s.messages = append(s.messages, fmt.Sprintf("提示：%s 未列出任何效果", potion.Slug))
		}

		isNew, err := isNewRecord(gormDB, &domain.Potion{}, "slug = ?", potion.Slug)
		if err != nil {
			s.fail(fmt.Sprintf("%s 预查失败: %v", potion.Slug, err))
			continue
		}
		if dryRun {
			fmt.Printf("[dry-run] %s potion %s (%s)\n", upsertVerb(isNew), potion.Slug, potion.NameZh)
			s.countOutcome(isNew)
			continue
		}
		if err := gormDB.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "slug"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"name_zh", "name_en", "aliases", "category", "summary", "effect_note",
				"is_brewable", "is_extendable", "is_enhanceable", "effects", "game_edition",
				"supported_versions", "source_url", "data_version", "last_verified_at", "updated_at",
			}),
		}).Create(potion).Error; err != nil {
			s.fail(fmt.Sprintf("%s 导入失败: %v", row.Slug, err))
			continue
		}
		s.countOutcome(isNew)
	}

	// ---- 酿造步骤 ----
	for i, row := range file.BrewingRecipes {
		step, err := buildBrewingRecipe(row, knownSlugs)
		if err != nil {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("酿造第 %d 行：%v", i+1, err))
			continue
		}
		isNew, err := isNewRecord(gormDB, &domain.BrewingRecipe{}, "recipe_key = ?", step.RecipeKey)
		if err != nil {
			s.fail(fmt.Sprintf("%s 预查失败: %v", step.RecipeKey, err))
			continue
		}
		if dryRun {
			fmt.Printf("[dry-run] %s brewing %s\n", upsertVerb(isNew), step.RecipeKey)
			s.countOutcome(isNew)
			continue
		}
		if err := gormDB.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "recipe_key"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"step_kind", "input_potion_slug", "ingredient_item_id", "ingredient_name_zh",
				"result_potion_slug", "note", "game_edition", "supported_versions",
				"source_url", "sort_order", "updated_at",
			}),
		}).Create(step).Error; err != nil {
			s.fail(fmt.Sprintf("%s 导入失败: %v", row.RecipeKey, err))
			continue
		}
		s.countOutcome(isNew)
	}
	return s
}

// aliasOrEmpty 归一化别名为非 nil 切片。
func aliasOrEmpty(aliases []string) []string {
	if aliases == nil {
		return []string{}
	}
	return aliases
}

// boolOr 取指针布尔值，nil 时返回默认值。
func boolOr(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}
