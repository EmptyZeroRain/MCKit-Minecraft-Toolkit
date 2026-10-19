// 生物资料导入（TODO §9C）。
//
// 数据文件为单个 JSON 对象：{ "mobs": [...], "drops": [...] }。
// 校验策略（对应 §9「校验资料来源和版本适用范围」）：
//   - mob：slug / name_zh / category / supported_versions / source_url 必填；
//     category 必须属于 passive|neutral|hostile|boss|utility|other；
//     dimensions 每项必须属于 overworld|nether|end；health 若给出须 ≥ 0。
//   - drop：drop_key / mob_slug / item_id / item_name_zh 必填；
//     mob_slug 必须能在同批数据或库中解析，否则视为悬空引用（致命）；
//     count_min ≥ 0 且 count_max ≥ count_min；chance ∈ [0,1]。
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

// mobSeedFile 生物资料文件结构。
type mobSeedFile struct {
	Mobs  []mobRow  `json:"mobs"`
	Drops []dropRow `json:"drops"`
}

// mobRow 生物导入原始行。
type mobRow struct {
	Slug              string   `json:"slug"`
	NameZh            string   `json:"name_zh"`
	NameEn            string   `json:"name_en"`
	Aliases           []string `json:"aliases"`
	Category          string   `json:"category"`
	Health            *float64 `json:"health"`
	AttackNote        string   `json:"attack_note"`
	BehaviorNote      string   `json:"behavior_note"`
	SpawnNote         string   `json:"spawn_note"`
	TamingNote        string   `json:"taming_note"`
	BreedingNote      string   `json:"breeding_note"`
	Dimensions        []string `json:"dimensions"`
	BiomesNote        string   `json:"biomes_note"`
	Summary           string   `json:"summary"`
	GameEdition       string   `json:"game_edition"`
	SupportedVersions []string `json:"supported_versions"`
	SourceURL         string   `json:"source_url"`
	DataVersion       string   `json:"data_version"`
	LastVerifiedAt    string   `json:"last_verified_at"`
	SortOrder         int      `json:"sort_order"`
}

// dropRow 掉落导入原始行。
type dropRow struct {
	DropKey    string  `json:"drop_key"`
	MobSlug    string  `json:"mob_slug"`
	ItemID     string  `json:"item_id"`
	ItemNameZh string  `json:"item_name_zh"`
	CountMin   int     `json:"count_min"`
	CountMax   int     `json:"count_max"`
	Chance     float64 `json:"chance"`
	Conditions string  `json:"conditions"`
	Note       string  `json:"note"`
	SortOrder  int     `json:"sort_order"`
}

// buildMob 校验并归一化单个生物。
func buildMob(row mobRow) (*domain.Mob, error) {
	slug := strings.TrimSpace(row.Slug)
	nameZh := strings.TrimSpace(row.NameZh)
	if slug == "" || nameZh == "" {
		return nil, errors.New("slug 与 name_zh 必填")
	}
	category := strings.TrimSpace(row.Category)
	if category == "" {
		category = domain.MobCategoryOther
	}
	if !domain.IsValidMobCategory(category) {
		return nil, fmt.Errorf("category 非法: %q", row.Category)
	}
	for i, d := range row.Dimensions {
		if !domain.IsValidMobDimension(strings.TrimSpace(d)) {
			return nil, fmt.Errorf("dimensions[%d] 非法: %q", i, d)
		}
	}
	if row.Health != nil && *row.Health < 0 {
		return nil, errors.New("health 不能为负")
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

	dims := make([]string, 0, len(row.Dimensions))
	for _, d := range row.Dimensions {
		dims = append(dims, strings.TrimSpace(d))
	}

	mob := &domain.Mob{
		Slug:              slug,
		NameZh:            nameZh,
		NameEn:            strings.TrimSpace(row.NameEn),
		Aliases:           types.JSONSlice(aliasOrEmpty(row.Aliases)),
		Category:          category,
		Health:            row.Health,
		AttackNote:        strings.TrimSpace(row.AttackNote),
		BehaviorNote:      strings.TrimSpace(row.BehaviorNote),
		SpawnNote:         strings.TrimSpace(row.SpawnNote),
		TamingNote:        strings.TrimSpace(row.TamingNote),
		BreedingNote:      strings.TrimSpace(row.BreedingNote),
		Dimensions:        types.JSONSlice(dims),
		BiomesNote:        strings.TrimSpace(row.BiomesNote),
		Summary:           strings.TrimSpace(row.Summary),
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
		mob.LastVerifiedAt = &t
	}
	return mob, nil
}

// buildMobDrop 校验并归一化单条掉落。
// knownMobs 为同批数据中的生物 slug 集合，用于检出悬空引用。
func buildMobDrop(row dropRow, knownMobs map[string]bool) (*domain.MobDrop, error) {
	key := strings.TrimSpace(row.DropKey)
	if key == "" {
		return nil, errors.New("drop_key 必填")
	}
	mobSlug := strings.TrimSpace(row.MobSlug)
	if mobSlug == "" {
		return nil, errors.New("mob_slug 必填")
	}
	if !knownMobs[mobSlug] {
		return nil, fmt.Errorf("mob_slug 未收录: %q", row.MobSlug)
	}
	itemID := strings.TrimSpace(row.ItemID)
	nameZh := strings.TrimSpace(row.ItemNameZh)
	if itemID == "" || nameZh == "" {
		return nil, errors.New("item_id 与 item_name_zh 必填")
	}
	if row.CountMin < 0 {
		return nil, errors.New("count_min 不能为负")
	}
	countMax := row.CountMax
	if countMax == 0 {
		countMax = row.CountMin
	}
	if countMax < row.CountMin {
		return nil, errors.New("count_max 不能小于 count_min")
	}
	chance := row.Chance
	if chance == 0 {
		// 未显式给出时视为「必定掉落」，避免把确定掉落误判为 0 概率。
		chance = 1
	}
	if chance < 0 || chance > 1 {
		return nil, errors.New("chance 必须在 0–1 之间")
	}

	return &domain.MobDrop{
		DropKey:    key,
		MobSlug:    mobSlug,
		ItemID:     itemID,
		ItemNameZh: nameZh,
		CountMin:   row.CountMin,
		CountMax:   countMax,
		Chance:     chance,
		Conditions: strings.TrimSpace(row.Conditions),
		Note:       strings.TrimSpace(row.Note),
		SortOrder:  row.SortOrder,
	}, nil
}

// importMobs 导入生物与掉落（先生物后掉落，便于引用校验）。
func importMobs(gormDB *gorm.DB, content []byte, dryRun bool) *summary {
	var file mobSeedFile
	if err := json.Unmarshal(content, &file); err != nil {
		fmt.Fprintf(os.Stderr, "JSON 解析失败: %v\n", err)
		os.Exit(1)
	}

	s := &summary{total: len(file.Mobs) + len(file.Drops)}
	knownMobs := make(map[string]bool, len(file.Mobs))

	// ---- 生物 ----
	for i, row := range file.Mobs {
		mob, err := buildMob(row)
		if err != nil {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("生物第 %d 行：%v", i+1, err))
			continue
		}
		knownMobs[mob.Slug] = true
		isNew, err := isNewRecord(gormDB, &domain.Mob{}, "slug = ?", mob.Slug)
		if err != nil {
			s.fail(fmt.Sprintf("%s 预查失败: %v", mob.Slug, err))
			continue
		}
		if dryRun {
			fmt.Printf("[dry-run] %s 生物 %s (%s)\n", upsertVerb(isNew), mob.Slug, mob.NameZh)
			s.countOutcome(isNew)
			continue
		}
		if err := gormDB.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "slug"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"name_zh", "name_en", "aliases", "category", "health", "attack_note",
				"behavior_note", "spawn_note", "taming_note", "breeding_note", "dimensions",
				"biomes_note", "summary", "game_edition", "supported_versions", "source_url",
				"data_version", "last_verified_at", "sort_order", "updated_at",
			}),
		}).Create(mob).Error; err != nil {
			s.fail(fmt.Sprintf("%s 导入失败: %v", mob.Slug, err))
			continue
		}
		s.countOutcome(isNew)
	}

	// ---- 掉落 ----
	for i, row := range file.Drops {
		drop, err := buildMobDrop(row, knownMobs)
		if err != nil {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("掉落第 %d 行：%v", i+1, err))
			continue
		}
		isNew, err := isNewRecord(gormDB, &domain.MobDrop{}, "drop_key = ?", drop.DropKey)
		if err != nil {
			s.fail(fmt.Sprintf("%s 预查失败: %v", drop.DropKey, err))
			continue
		}
		if dryRun {
			fmt.Printf("[dry-run] %s 掉落 %s\n", upsertVerb(isNew), drop.DropKey)
			s.countOutcome(isNew)
			continue
		}
		if err := gormDB.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "drop_key"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"mob_slug", "item_id", "item_name_zh", "count_min", "count_max",
				"chance", "conditions", "note", "sort_order", "updated_at",
			}),
		}).Create(drop).Error; err != nil {
			s.fail(fmt.Sprintf("%s 导入失败: %v", drop.DropKey, err))
			continue
		}
		s.countOutcome(isNew)
	}
	return s
}
