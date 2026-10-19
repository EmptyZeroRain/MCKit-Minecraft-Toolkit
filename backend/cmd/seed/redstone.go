// 红石与机器设计资料导入（TODO §15）。
//
// 数据文件为单个 JSON 对象：{ "components": [...], "designs": [...] }。
// 校验策略（对应 §15「保留来源与许可信息，建立内容审核流程」）：
//   - component：slug / name_zh 必填；category 必须属于
//     source|transmission|logic|actuator|sensor|other；
//     supported_versions 若给出须为「版本或范围」格式；source_url 缺失记为提醒（非阻断）。
//   - design：slug / name_zh / purpose / difficulty / supported_versions / source_url 必填；
//     purpose / difficulty / review_status 枚举合法；materials 每项 count_min ≥ 1 且
//     count_max ≥ count_min，item_id 为空时 name_zh 必填；
//     **world_download_url 非空时 license 必须非空**（未授权不得提供下载链接）。
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

// redstoneSeedFile 红石资料文件结构。
type redstoneSeedFile struct {
	Components []redstoneComponentRow `json:"components"`
	Designs    []redstoneDesignRow    `json:"designs"`
}

// redstoneComponentRow 红石元件导入原始行。
type redstoneComponentRow struct {
	Slug              string   `json:"slug"`
	NameZh            string   `json:"name_zh"`
	NameEn            string   `json:"name_en"`
	Aliases           []string `json:"aliases"`
	Category          string   `json:"category"`
	MechanismNote     string   `json:"mechanism_note"`
	SignalNote        string   `json:"signal_note"`
	Summary           string   `json:"summary"`
	GameEdition       string   `json:"game_edition"`
	SupportedVersions []string `json:"supported_versions"`
	SourceURL         string   `json:"source_url"`
	DataVersion       string   `json:"data_version"`
	LastVerifiedAt    string   `json:"last_verified_at"`
	SortOrder         int      `json:"sort_order"`
}

// redstoneMaterialRow 材料导入原始行。
type redstoneMaterialRow struct {
	ItemID   string `json:"item_id"`
	NameZh   string `json:"name_zh"`
	CountMin int    `json:"count_min"`
	CountMax int    `json:"count_max"`
}

// redstoneDesignRow 机器设计导入原始行。
type redstoneDesignRow struct {
	Slug              string                `json:"slug"`
	NameZh            string                `json:"name_zh"`
	NameEn            string                `json:"name_en"`
	Aliases           []string              `json:"aliases"`
	Purpose           string                `json:"purpose"`
	Difficulty        string                `json:"difficulty"`
	Summary           string                `json:"summary"`
	StepsMd           string                `json:"steps_md"`
	Materials         []redstoneMaterialRow `json:"materials"`
	Author            string                `json:"author"`
	AuthorURL         string                `json:"author_url"`
	ScreenshotURL     string                `json:"screenshot_url"`
	VideoURL          string                `json:"video_url"`
	WorldDownloadURL  string                `json:"world_download_url"`
	WorldDownloadNote string                `json:"world_download_note"`
	License           string                `json:"license"`
	LicenseNote       string                `json:"license_note"`
	SupportedVersions []string              `json:"supported_versions"`
	GameEdition       string                `json:"game_edition"`
	SourceURL         string                `json:"source_url"`
	DataVersion       string                `json:"data_version"`
	LastVerifiedAt    string                `json:"last_verified_at"`
	ReviewStatus      string                `json:"review_status"`
	Reviewer          string                `json:"reviewer"`
	ReviewedAt        string                `json:"reviewed_at"`
	ReviewNote        string                `json:"review_note"`
	SortOrder         int                   `json:"sort_order"`
}

// buildRedstoneComponent 校验并归一化单个红石元件。
func buildRedstoneComponent(row redstoneComponentRow) (*domain.RedstoneComponent, error) {
	slug := strings.TrimSpace(row.Slug)
	nameZh := strings.TrimSpace(row.NameZh)
	if slug == "" || nameZh == "" {
		return nil, errors.New("slug 与 name_zh 必填")
	}
	category := strings.TrimSpace(row.Category)
	if category == "" {
		category = domain.RedstoneComponentCategoryOther
	}
	if !domain.IsValidRedstoneComponentCategory(category) {
		return nil, fmt.Errorf("category 非法: %q", row.Category)
	}
	if err := validateVersions("supported_versions", row.SupportedVersions); err != nil {
		return nil, err
	}
	edition := strings.TrimSpace(row.GameEdition)
	if edition == "" {
		edition = "java"
	}
	if !validEditions[edition] {
		return nil, fmt.Errorf("game_edition 非法: %q", row.GameEdition)
	}

	component := &domain.RedstoneComponent{
		Slug:              slug,
		NameZh:            nameZh,
		NameEn:            strings.TrimSpace(row.NameEn),
		Aliases:           types.JSONSlice(aliasOrEmpty(row.Aliases)),
		Category:          category,
		MechanismNote:     strings.TrimSpace(row.MechanismNote),
		SignalNote:        strings.TrimSpace(row.SignalNote),
		Summary:           strings.TrimSpace(row.Summary),
		GameEdition:       edition,
		SupportedVersions: types.JSONSlice(versioning.Sort(row.SupportedVersions)),
		SourceURL:         strings.TrimSpace(row.SourceURL),
		DataVersion:       strings.TrimSpace(row.DataVersion),
		SortOrder:         row.SortOrder,
	}
	if raw := strings.TrimSpace(row.LastVerifiedAt); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, fmt.Errorf("last_verified_at 需为 RFC3339 时间: %v", err)
		}
		component.LastVerifiedAt = &t
	}
	return component, nil
}

// buildRedstoneMaterial 校验并归一化单条材料。
func buildRedstoneMaterial(row redstoneMaterialRow) (domain.RedstoneMaterial, error) {
	itemID := strings.TrimSpace(row.ItemID)
	nameZh := strings.TrimSpace(row.NameZh)
	if itemID == "" && nameZh == "" {
		return domain.RedstoneMaterial{}, errors.New("item_id 为空时 name_zh 必填")
	}
	if row.CountMin < 1 {
		return domain.RedstoneMaterial{}, errors.New("count_min 必须 ≥ 1")
	}
	countMax := row.CountMax
	if countMax == 0 {
		countMax = row.CountMin
	}
	if countMax < row.CountMin {
		return domain.RedstoneMaterial{}, errors.New("count_max 不能小于 count_min")
	}
	return domain.RedstoneMaterial{
		ItemID:   itemID,
		NameZh:   nameZh,
		CountMin: row.CountMin,
		CountMax: countMax,
	}, nil
}

// buildRedstoneDesign 校验并归一化单个机器设计。
func buildRedstoneDesign(row redstoneDesignRow) (*domain.RedstoneDesign, error) {
	slug := strings.TrimSpace(row.Slug)
	nameZh := strings.TrimSpace(row.NameZh)
	if slug == "" || nameZh == "" {
		return nil, errors.New("slug 与 name_zh 必填")
	}
	purpose := strings.TrimSpace(row.Purpose)
	if purpose == "" {
		purpose = domain.RedstoneDesignPurposeOther
	}
	if !domain.IsValidRedstoneDesignPurpose(purpose) {
		return nil, fmt.Errorf("purpose 非法: %q", row.Purpose)
	}
	difficulty := strings.TrimSpace(row.Difficulty)
	if difficulty == "" {
		difficulty = domain.RedstoneDesignDifficultyMedium
	}
	if !domain.IsValidRedstoneDesignDifficulty(difficulty) {
		return nil, fmt.Errorf("difficulty 非法: %q", row.Difficulty)
	}
	reviewStatus := strings.TrimSpace(row.ReviewStatus)
	if reviewStatus == "" {
		reviewStatus = domain.RedstoneReviewPending
	}
	if !domain.IsValidRedstoneReviewStatus(reviewStatus) {
		return nil, fmt.Errorf("review_status 非法: %q", row.ReviewStatus)
	}
	sourceURL := strings.TrimSpace(row.SourceURL)
	if sourceURL == "" {
		return nil, errors.New("source_url 必填（需明确资料来源）")
	}
	if len(row.SupportedVersions) == 0 {
		return nil, errors.New("supported_versions 必填（需明确版本适用范围）")
	}
	if err := validateVersions("supported_versions", row.SupportedVersions); err != nil {
		return nil, err
	}
	edition := strings.TrimSpace(row.GameEdition)
	if edition == "" {
		edition = "java"
	}
	if !validEditions[edition] {
		return nil, fmt.Errorf("game_edition 非法: %q", row.GameEdition)
	}

	// 版权红线：未授权存档一律不提供下载链接。
	worldURL := strings.TrimSpace(row.WorldDownloadURL)
	license := strings.TrimSpace(row.License)
	if worldURL != "" && license == "" {
		return nil, errors.New("world_download_url 非空时 license 必填（未授权不得提供存档下载）")
	}

	materials := make(domain.RedstoneMaterials, 0, len(row.Materials))
	for i, m := range row.Materials {
		mat, err := buildRedstoneMaterial(m)
		if err != nil {
			return nil, fmt.Errorf("materials[%d]：%v", i, err)
		}
		materials = append(materials, mat)
	}

	design := &domain.RedstoneDesign{
		Slug:              slug,
		NameZh:            nameZh,
		NameEn:            strings.TrimSpace(row.NameEn),
		Aliases:           types.JSONSlice(aliasOrEmpty(row.Aliases)),
		Purpose:           purpose,
		Difficulty:        difficulty,
		Summary:           strings.TrimSpace(row.Summary),
		StepsMd:           strings.TrimSpace(row.StepsMd),
		Materials:         materials,
		Author:            strings.TrimSpace(row.Author),
		AuthorURL:         strings.TrimSpace(row.AuthorURL),
		ScreenshotURL:     strings.TrimSpace(row.ScreenshotURL),
		VideoURL:          strings.TrimSpace(row.VideoURL),
		WorldDownloadURL:  worldURL,
		WorldDownloadNote: strings.TrimSpace(row.WorldDownloadNote),
		License:           license,
		LicenseNote:       strings.TrimSpace(row.LicenseNote),
		SupportedVersions: types.JSONSlice(versioning.Sort(row.SupportedVersions)),
		GameEdition:       edition,
		SourceURL:         sourceURL,
		DataVersion:       strings.TrimSpace(row.DataVersion),
		ReviewStatus:      reviewStatus,
		Reviewer:          strings.TrimSpace(row.Reviewer),
		ReviewNote:        strings.TrimSpace(row.ReviewNote),
		SortOrder:         row.SortOrder,
	}
	if raw := strings.TrimSpace(row.LastVerifiedAt); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, fmt.Errorf("last_verified_at 需为 RFC3339 时间: %v", err)
		}
		design.LastVerifiedAt = &t
	}
	if raw := strings.TrimSpace(row.ReviewedAt); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, fmt.Errorf("reviewed_at 需为 RFC3339 时间: %v", err)
		}
		design.ReviewedAt = &t
	}
	return design, nil
}

// importRedstone 导入红石元件与机器设计（先元件后设计，便于引用校验）。
func importRedstone(gormDB *gorm.DB, content []byte, dryRun bool) *summary {
	var file redstoneSeedFile
	if err := json.Unmarshal(content, &file); err != nil {
		fmt.Fprintf(os.Stderr, "JSON 解析失败: %v\n", err)
		os.Exit(1)
	}

	s := &summary{total: len(file.Components) + len(file.Designs)}

	// ---- 红石元件 ----
	for i, row := range file.Components {
		component, err := buildRedstoneComponent(row)
		if err != nil {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("元件第 %d 行：%v", i+1, err))
			continue
		}
		if component.SourceURL == "" {
			s.messages = append(s.messages, fmt.Sprintf("提示：元件 %s 缺少 source_url", component.Slug))
		}
		isNew, err := isNewRecord(gormDB, &domain.RedstoneComponent{}, "slug = ?", component.Slug)
		if err != nil {
			s.fail(fmt.Sprintf("%s 预查失败: %v", component.Slug, err))
			continue
		}
		if dryRun {
			fmt.Printf("[dry-run] %s 元件 %s (%s)\n", upsertVerb(isNew), component.Slug, component.NameZh)
			s.countOutcome(isNew)
			continue
		}
		if err := gormDB.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "slug"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"name_zh", "name_en", "aliases", "category", "mechanism_note", "signal_note",
				"summary", "game_edition", "supported_versions", "source_url", "data_version",
				"last_verified_at", "sort_order", "updated_at",
			}),
		}).Create(component).Error; err != nil {
			s.fail(fmt.Sprintf("%s 导入失败: %v", component.Slug, err))
			continue
		}
		s.countOutcome(isNew)
	}

	// ---- 机器设计 ----
	for i, row := range file.Designs {
		design, err := buildRedstoneDesign(row)
		if err != nil {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("设计第 %d 行：%v", i+1, err))
			continue
		}
		isNew, err := isNewRecord(gormDB, &domain.RedstoneDesign{}, "slug = ?", design.Slug)
		if err != nil {
			s.fail(fmt.Sprintf("%s 预查失败: %v", design.Slug, err))
			continue
		}
		if dryRun {
			fmt.Printf("[dry-run] %s 设计 %s (%s)\n", upsertVerb(isNew), design.Slug, design.NameZh)
			s.countOutcome(isNew)
			continue
		}
		if err := gormDB.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "slug"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"name_zh", "name_en", "aliases", "purpose", "difficulty", "summary", "steps_md",
				"materials", "author", "author_url", "screenshot_url", "video_url",
				"world_download_url", "world_download_note", "license", "license_note",
				"supported_versions", "game_edition", "source_url", "data_version",
				"last_verified_at", "review_status", "reviewer", "reviewed_at", "review_note",
				"sort_order", "updated_at",
			}),
		}).Create(design).Error; err != nil {
			s.fail(fmt.Sprintf("%s 导入失败: %v", design.Slug, err))
			continue
		}
		s.countOutcome(isNew)
	}
	return s
}
