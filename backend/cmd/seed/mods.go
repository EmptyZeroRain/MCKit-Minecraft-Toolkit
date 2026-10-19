// Mod 与整合包兼容性导入（TODO §16）。
//
// 数据文件为单个 JSON 对象：{ "mods": [...], "modpacks": [...] }；
// 其中 mod 内嵌 versions 与 dependencies，modpack 内嵌 mods。
//
// 校验策略（对应 §16「为兼容性结论标注数据来源和验证状态」）：
//   - mod：slug / name_zh 必填；categories / loaders 枚举合法；mc_versions 版本格式合法；
//     source_url 必填（可核验锚点）；review_status 枚举合法。
//   - version：version 必填；release_type 枚举合法；mc_versions / loaders 合法；
//     **download_url 非空时 license 或 license_note 必须说明来源授权**（不未经许可镜像分发）。
//   - dependency：dep_kind 枚举合法；**incompatible 必须给出 note**；
//     target_mod_slug 或 target_mod_name 必填其一；target_mod_slug 给出但无法解析 → 悬空引用致命失败。
//   - modpack：slug / name_zh / source_url 必填；loader / review_status 枚举合法。
//   - modpack_mod：mod_slug 或 mod_name 必填其一；mod_slug 给出但无法解析 → 悬空引用致命失败。
//
// 幂等策略：mods / modpacks 按 slug 冲突更新；mod_versions 按 (mod_id, version) 冲突更新
// 并清理该 Mod 下已删除的版本；mod_dependencies / modpack_mods 按父记录整体重建（净效果幂等）。
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

// modsSeedFile Mod 与整合包文件结构。
type modsSeedFile struct {
	Mods     []modRow     `json:"mods"`
	Modpacks []modpackRow `json:"modpacks"`
}

// modRow Mod 导入原始行（含内嵌发布版本与依赖）。
type modRow struct {
	Slug           string             `json:"slug"`
	NameZh         string             `json:"name_zh"`
	NameEn         string             `json:"name_en"`
	Aliases        []string           `json:"aliases"`
	Summary        string             `json:"summary"`
	Description    string             `json:"description"`
	Categories     []string           `json:"categories"`
	Loaders        []string           `json:"loaders"`
	MCVersions     []string           `json:"mc_versions"`
	HomepageURL    string             `json:"homepage_url"`
	SourceURL      string             `json:"source_url"`
	License        string             `json:"license"`
	LicenseNote    string             `json:"license_note"`
	GameEdition    string             `json:"game_edition"`
	DataVersion    string             `json:"data_version"`
	LastVerifiedAt string             `json:"last_verified_at"`
	ReviewStatus   string             `json:"review_status"`
	Reviewer       string             `json:"reviewer"`
	ReviewedAt     string             `json:"reviewed_at"`
	ReviewNote     string             `json:"review_note"`
	SortOrder      int                `json:"sort_order"`
	Versions       []modVersionRow    `json:"versions"`
	Dependencies   []modDependencyRow `json:"dependencies"`
}

// modVersionRow 发布版本导入原始行。
type modVersionRow struct {
	Version      string   `json:"version"`
	ReleaseType  string   `json:"release_type"`
	MCVersions   []string `json:"mc_versions"`
	Loaders      []string `json:"loaders"`
	DownloadURL  string   `json:"download_url"`
	DownloadNote string   `json:"download_note"`
	FileName     string   `json:"file_name"`
	PublishedAt  string   `json:"published_at"`
	SourceURL    string   `json:"source_url"`
	Verified     bool     `json:"verified"`
	VerifyNote   string   `json:"verify_note"`
	SortOrder    int      `json:"sort_order"`
}

// modDependencyRow 依赖 / 冲突导入原始行。
type modDependencyRow struct {
	DepKind       string `json:"dep_kind"`
	TargetModSlug string `json:"target_mod_slug"`
	TargetModName string `json:"target_mod_name"`
	VersionNote   string `json:"version_note"`
	Note          string `json:"note"`
	SourceURL     string `json:"source_url"`
	Verified      bool   `json:"verified"`
}

// modpackRow 整合包导入原始行（含内嵌 Mod 清单）。
type modpackRow struct {
	Slug           string          `json:"slug"`
	NameZh         string          `json:"name_zh"`
	NameEn         string          `json:"name_en"`
	Aliases        []string        `json:"aliases"`
	Summary        string          `json:"summary"`
	Description    string          `json:"description"`
	MCVersion      string          `json:"mc_version"`
	Loader         string          `json:"loader"`
	Author         string          `json:"author"`
	HomepageURL    string          `json:"homepage_url"`
	SourceURL      string          `json:"source_url"`
	License        string          `json:"license"`
	LicenseNote    string          `json:"license_note"`
	DataVersion    string          `json:"data_version"`
	LastVerifiedAt string          `json:"last_verified_at"`
	ReviewStatus   string          `json:"review_status"`
	Reviewer       string          `json:"reviewer"`
	ReviewedAt     string          `json:"reviewed_at"`
	ReviewNote     string          `json:"review_note"`
	SortOrder      int             `json:"sort_order"`
	Mods           []modpackModRow `json:"mods"`
}

// modpackModRow 整合包内 Mod 清单导入原始行。
type modpackModRow struct {
	ModSlug   string `json:"mod_slug"`
	ModName   string `json:"mod_name"`
	Version   string `json:"version"`
	Note      string `json:"note"`
	SortOrder int    `json:"sort_order"`
}

// validateLoaderList 校验加载器列表，返回聚合错误。
func validateLoaderList(loaders []string) error {
	for _, l := range loaders {
		if !domain.IsValidModLoader(strings.TrimSpace(l)) {
			return fmt.Errorf("loader 非法: %q", l)
		}
	}
	return nil
}

// validateCategoryList 校验类别列表，返回聚合错误。
func validateCategoryList(categories []string) error {
	for _, c := range categories {
		if !domain.IsValidModCategory(strings.TrimSpace(c)) {
			return fmt.Errorf("category 非法: %q", c)
		}
	}
	return nil
}

// parseOptionalTime 解析可选的 RFC3339 时间。
func parseOptionalTime(label, raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, fmt.Errorf("%s 需为 RFC3339 时间: %v", label, err)
	}
	return &t, nil
}

// buildMod 校验并归一化单个 Mod（不含子表）。
func buildMod(row modRow) (*domain.Mod, error) {
	slug := strings.TrimSpace(row.Slug)
	nameZh := strings.TrimSpace(row.NameZh)
	if slug == "" || nameZh == "" {
		return nil, errors.New("slug 与 name_zh 必填")
	}
	if err := validateCategoryList(row.Categories); err != nil {
		return nil, err
	}
	if err := validateLoaderList(row.Loaders); err != nil {
		return nil, err
	}
	if err := validateVersions("mc_versions", row.MCVersions); err != nil {
		return nil, err
	}
	sourceURL := strings.TrimSpace(row.SourceURL)
	if sourceURL == "" {
		return nil, errors.New("source_url 必填（需明确可核验的官方来源）")
	}
	reviewStatus := strings.TrimSpace(row.ReviewStatus)
	if reviewStatus == "" {
		reviewStatus = domain.ModReviewPending
	}
	if !domain.IsValidModReviewStatus(reviewStatus) {
		return nil, fmt.Errorf("review_status 非法: %q", row.ReviewStatus)
	}
	edition := strings.TrimSpace(row.GameEdition)
	if edition == "" {
		edition = "java"
	}
	if !validEditions[edition] {
		return nil, fmt.Errorf("game_edition 非法: %q", row.GameEdition)
	}

	m := &domain.Mod{
		Slug:         slug,
		NameZh:       nameZh,
		NameEn:       strings.TrimSpace(row.NameEn),
		Aliases:      types.JSONSlice(aliasOrEmpty(row.Aliases)),
		Summary:      strings.TrimSpace(row.Summary),
		Description:  strings.TrimSpace(row.Description),
		Categories:   types.JSONSlice(stringsOrEmpty(row.Categories)),
		Loaders:      types.JSONSlice(stringsOrEmpty(row.Loaders)),
		MCVersions:   types.JSONSlice(versioning.Sort(row.MCVersions)),
		HomepageURL:  strings.TrimSpace(row.HomepageURL),
		SourceURL:    sourceURL,
		License:      strings.TrimSpace(row.License),
		LicenseNote:  strings.TrimSpace(row.LicenseNote),
		GameEdition:  edition,
		DataVersion:  strings.TrimSpace(row.DataVersion),
		ReviewStatus: reviewStatus,
		Reviewer:     strings.TrimSpace(row.Reviewer),
		ReviewNote:   strings.TrimSpace(row.ReviewNote),
		SortOrder:    row.SortOrder,
	}
	t, err := parseOptionalTime("last_verified_at", row.LastVerifiedAt)
	if err != nil {
		return nil, err
	}
	m.LastVerifiedAt = t
	rt, err := parseOptionalTime("reviewed_at", row.ReviewedAt)
	if err != nil {
		return nil, err
	}
	m.ReviewedAt = rt
	return m, nil
}

// buildModVersion 校验并归一化单个发布版本。
// license / licenseNote 来自所属 Mod，用于校验下载链接的授权说明（不镜像分发红线）。
func buildModVersion(row modVersionRow, license, licenseNote string) (*domain.ModVersion, error) {
	version := strings.TrimSpace(row.Version)
	if version == "" {
		return nil, errors.New("version 必填")
	}
	releaseType := strings.TrimSpace(row.ReleaseType)
	if releaseType == "" {
		releaseType = domain.ModReleaseTypeRelease
	}
	if !domain.IsValidModReleaseType(releaseType) {
		return nil, fmt.Errorf("release_type 非法: %q", row.ReleaseType)
	}
	if err := validateVersions("mc_versions", row.MCVersions); err != nil {
		return nil, err
	}
	if err := validateLoaderList(row.Loaders); err != nil {
		return nil, err
	}
	downloadURL := strings.TrimSpace(row.DownloadURL)
	// 版权红线：下载链接非空时必须有授权说明（license 或 license_note）。
	if downloadURL != "" && strings.TrimSpace(license) == "" && strings.TrimSpace(licenseNote) == "" {
		return nil, errors.New("download_url 非空时 license 或 license_note 必须说明来源授权（不未经许可镜像分发）")
	}
	v := &domain.ModVersion{
		Version:      version,
		ReleaseType:  releaseType,
		MCVersions:   types.JSONSlice(versioning.Sort(row.MCVersions)),
		Loaders:      types.JSONSlice(stringsOrEmpty(row.Loaders)),
		DownloadURL:  downloadURL,
		DownloadNote: strings.TrimSpace(row.DownloadNote),
		FileName:     strings.TrimSpace(row.FileName),
		SourceURL:    strings.TrimSpace(row.SourceURL),
		Verified:     row.Verified,
		VerifyNote:   strings.TrimSpace(row.VerifyNote),
		SortOrder:    row.SortOrder,
	}
	t, err := parseOptionalTime("published_at", row.PublishedAt)
	if err != nil {
		return nil, err
	}
	v.PublishedAt = t
	return v, nil
}

// buildModDependency 校验并归一化单条依赖 / 冲突。
func buildModDependency(row modDependencyRow) (*domain.ModDependency, error) {
	depKind := strings.TrimSpace(row.DepKind)
	if !domain.IsValidModDependencyKind(depKind) {
		return nil, fmt.Errorf("dep_kind 非法: %q", row.DepKind)
	}
	targetSlug := strings.TrimSpace(row.TargetModSlug)
	targetName := strings.TrimSpace(row.TargetModName)
	if targetSlug == "" && targetName == "" {
		return nil, errors.New("target_mod_slug 或 target_mod_name 必填其一")
	}
	note := strings.TrimSpace(row.Note)
	// 不兼容关系必须给出依据说明，避免无据的「冲突」结论。
	if depKind == domain.ModDependencyIncompatible && note == "" {
		return nil, errors.New("incompatible 依赖必须给出 note（说明冲突依据）")
	}
	return &domain.ModDependency{
		DepKind:       depKind,
		TargetModName: targetName,
		VersionNote:   strings.TrimSpace(row.VersionNote),
		Note:          note,
		SourceURL:     strings.TrimSpace(row.SourceURL),
		Verified:      row.Verified,
	}, nil
}

// buildModpack 校验并归一化单个整合包（不含 Mod 清单）。
func buildModpack(row modpackRow) (*domain.Modpack, error) {
	slug := strings.TrimSpace(row.Slug)
	nameZh := strings.TrimSpace(row.NameZh)
	if slug == "" || nameZh == "" {
		return nil, errors.New("slug 与 name_zh 必填")
	}
	sourceURL := strings.TrimSpace(row.SourceURL)
	if sourceURL == "" {
		return nil, errors.New("source_url 必填（需明确可核验的官方来源）")
	}
	loader := strings.TrimSpace(row.Loader)
	if loader != "" && !domain.IsValidModLoader(loader) {
		return nil, fmt.Errorf("loader 非法: %q", row.Loader)
	}
	mcVersion := strings.TrimSpace(row.MCVersion)
	if mcVersion != "" && !versioning.IsValid(mcVersion) {
		return nil, fmt.Errorf("mc_version 格式非法: %q", row.MCVersion)
	}
	reviewStatus := strings.TrimSpace(row.ReviewStatus)
	if reviewStatus == "" {
		reviewStatus = domain.ModReviewPending
	}
	if !domain.IsValidModReviewStatus(reviewStatus) {
		return nil, fmt.Errorf("review_status 非法: %q", row.ReviewStatus)
	}

	pack := &domain.Modpack{
		Slug:         slug,
		NameZh:       nameZh,
		NameEn:       strings.TrimSpace(row.NameEn),
		Aliases:      types.JSONSlice(aliasOrEmpty(row.Aliases)),
		Summary:      strings.TrimSpace(row.Summary),
		Description:  strings.TrimSpace(row.Description),
		MCVersion:    mcVersion,
		Loader:       loader,
		Author:       strings.TrimSpace(row.Author),
		HomepageURL:  strings.TrimSpace(row.HomepageURL),
		SourceURL:    sourceURL,
		License:      strings.TrimSpace(row.License),
		LicenseNote:  strings.TrimSpace(row.LicenseNote),
		DataVersion:  strings.TrimSpace(row.DataVersion),
		ReviewStatus: reviewStatus,
		Reviewer:     strings.TrimSpace(row.Reviewer),
		ReviewNote:   strings.TrimSpace(row.ReviewNote),
		SortOrder:    row.SortOrder,
	}
	t, err := parseOptionalTime("last_verified_at", row.LastVerifiedAt)
	if err != nil {
		return nil, err
	}
	pack.LastVerifiedAt = t
	rt, err := parseOptionalTime("reviewed_at", row.ReviewedAt)
	if err != nil {
		return nil, err
	}
	pack.ReviewedAt = rt
	return pack, nil
}

// buildModpackMod 校验并归一化整合包内一条 Mod 清单项。
func buildModpackMod(row modpackModRow) (*domain.ModpackMod, error) {
	modSlug := strings.TrimSpace(row.ModSlug)
	modName := strings.TrimSpace(row.ModName)
	if modSlug == "" && modName == "" {
		return nil, errors.New("mod_slug 或 mod_name 必填其一")
	}
	return &domain.ModpackMod{
		ModName:   modName,
		Version:   strings.TrimSpace(row.Version),
		Note:      strings.TrimSpace(row.Note),
		SortOrder: row.SortOrder,
	}, nil
}

// modEntry 一个校验通过的 Mod 及其子记录。
type modEntry struct {
	mod      *domain.Mod
	versions []domain.ModVersion
	deps     []struct {
		dep   domain.ModDependency
		slug  string // target_mod_slug（可能为空）
		rowNo int
	}
	rowNo int
}

// modpackEntry 一个校验通过的整合包及其 Mod 清单。
type modpackEntry struct {
	pack  *domain.Modpack
	mods  []domain.ModpackMod
	slugs []string // 每项对应的 mod_slug（可能为空）
}

// importMods 导入 Mod 与整合包（TODO §16）。
func importMods(gormDB *gorm.DB, content []byte, dryRun bool) *summary {
	var file modsSeedFile
	if err := json.Unmarshal(content, &file); err != nil {
		fmt.Fprintf(os.Stderr, "JSON 解析失败: %v\n", err)
		os.Exit(1)
	}

	s := &summary{total: len(file.Mods) + len(file.Modpacks)}

	// ---- 第一遍：逐行校验并构造（失败逐行汇总，不中断）----
	entries := make([]modEntry, 0, len(file.Mods))
	fileSlugs := make(map[string]struct{}, len(file.Mods))
	for i, row := range file.Mods {
		m, err := buildMod(row)
		if err != nil {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("Mod 第 %d 行：%v", i+1, err))
			continue
		}
		entry := modEntry{mod: m, rowNo: i + 1}
		bad := false
		for j, vrow := range row.Versions {
			v, err := buildModVersion(vrow, m.License, m.LicenseNote)
			if err != nil {
				s.failed++
				s.messages = append(s.messages, fmt.Sprintf("Mod %s 版本第 %d 行：%v", m.Slug, j+1, err))
				bad = true
				continue
			}
			entry.versions = append(entry.versions, *v)
		}
		for j, drow := range row.Dependencies {
			d, err := buildModDependency(drow)
			if err != nil {
				s.failed++
				s.messages = append(s.messages, fmt.Sprintf("Mod %s 依赖第 %d 行：%v", m.Slug, j+1, err))
				bad = true
				continue
			}
			entry.deps = append(entry.deps, struct {
				dep   domain.ModDependency
				slug  string
				rowNo int
			}{dep: *d, slug: strings.TrimSpace(drow.TargetModSlug), rowNo: j + 1})
		}
		if bad {
			continue
		}
		entries = append(entries, entry)
		fileSlugs[m.Slug] = struct{}{}
	}

	// ---- 悬空引用校验：收集所有被引用的 slug，查库确认存在性 ----
	referenced := make(map[string]struct{})
	for _, e := range entries {
		for _, d := range e.deps {
			if d.slug != "" {
				referenced[d.slug] = struct{}{}
			}
		}
	}
	for _, row := range file.Modpacks {
		for _, mr := range row.Mods {
			if sl := strings.TrimSpace(mr.ModSlug); sl != "" {
				referenced[sl] = struct{}{}
			}
		}
	}
	dbSlugs, err := existingModSlugs(gormDB, referenced)
	if err != nil {
		s.fail(fmt.Sprintf("Mod slug 存在性预查失败: %v", err))
		dbSlugs = map[string]struct{}{}
	}
	known := func(slug string) bool {
		if _, ok := fileSlugs[slug]; ok {
			return true
		}
		_, ok := dbSlugs[slug]
		return ok
	}

	// ---- 第二遍：校验引用可解析（悬空引用致命失败，跳过该 Mod）----
	valid := entries[:0]
	for _, e := range entries {
		dangling := false
		for _, d := range e.deps {
			if d.slug != "" && !known(d.slug) {
				s.failed++
				s.messages = append(s.messages, fmt.Sprintf(
					"Mod %s 依赖第 %d 行：target_mod_slug %q 无法解析（悬空引用）", e.mod.Slug, d.rowNo, d.slug))
				dangling = true
			}
		}
		if dangling {
			continue
		}
		valid = append(valid, e)
	}
	entries = valid

	// ---- 写入 Mod 与发布版本（依赖留待全部 Mod 落库后再写）----
	written := make([]modEntry, 0, len(entries))
	for _, e := range entries {
		isNew, err := isNewRecord(gormDB, &domain.Mod{}, "slug = ?", e.mod.Slug)
		if err != nil {
			s.fail(fmt.Sprintf("%s 预查失败: %v", e.mod.Slug, err))
			continue
		}
		if dryRun {
			fmt.Printf("[dry-run] %s Mod %s (%s)，版本 %d 条，依赖 %d 条\n",
				upsertVerb(isNew), e.mod.Slug, e.mod.NameZh, len(e.versions), len(e.deps))
			s.countOutcome(isNew)
			continue
		}
		if err := upsertModBase(gormDB, e); err != nil {
			s.fail(fmt.Sprintf("%s 导入失败: %v", e.mod.Slug, err))
			continue
		}
		written = append(written, e)
		s.countOutcome(isNew)
	}

	// ---- 写入依赖（此时全部 Mod 已落库，可安全解析 target_mod_slug）----
	if !dryRun {
		for _, e := range written {
			if err := upsertModDeps(gormDB, e); err != nil {
				s.fail(fmt.Sprintf("%s 依赖导入失败: %v", e.mod.Slug, err))
			}
		}
	}

	// ---- 写入整合包 ----
	for i, row := range file.Modpacks {
		pack, err := buildModpack(row)
		if err != nil {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("整合包第 %d 行：%v", i+1, err))
			continue
		}
		entry := modpackEntry{pack: pack}
		bad := false
		for j, mr := range row.Mods {
			pm, err := buildModpackMod(mr)
			if err != nil {
				s.failed++
				s.messages = append(s.messages, fmt.Sprintf("整合包 %s 清单第 %d 行：%v", pack.Slug, j+1, err))
				bad = true
				continue
			}
			slug := strings.TrimSpace(mr.ModSlug)
			if slug != "" && !known(slug) {
				s.failed++
				s.messages = append(s.messages, fmt.Sprintf(
					"整合包 %s 清单第 %d 行：mod_slug %q 无法解析（悬空引用）", pack.Slug, j+1, slug))
				bad = true
				continue
			}
			entry.mods = append(entry.mods, *pm)
			entry.slugs = append(entry.slugs, slug)
		}
		if bad {
			continue
		}

		isNew, err := isNewRecord(gormDB, &domain.Modpack{}, "slug = ?", pack.Slug)
		if err != nil {
			s.fail(fmt.Sprintf("%s 预查失败: %v", pack.Slug, err))
			continue
		}
		if dryRun {
			fmt.Printf("[dry-run] %s 整合包 %s (%s)，Mod %d 项\n",
				upsertVerb(isNew), pack.Slug, pack.NameZh, len(entry.mods))
			s.countOutcome(isNew)
			continue
		}
		if err := upsertModpack(gormDB, entry); err != nil {
			s.fail(fmt.Sprintf("%s 导入失败: %v", pack.Slug, err))
			continue
		}
		s.countOutcome(isNew)
	}
	return s
}

// existingModSlugs 查询给定 slug 集合中已存在于 mods 表的子集。
func existingModSlugs(gormDB *gorm.DB, slugs map[string]struct{}) (map[string]struct{}, error) {
	out := make(map[string]struct{})
	if len(slugs) == 0 {
		return out, nil
	}
	list := make([]string, 0, len(slugs))
	for sl := range slugs {
		list = append(list, sl)
	}
	var rows []domain.Mod
	if err := gormDB.Model(&domain.Mod{}).Select("slug").Where("slug IN ?", list).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, m := range rows {
		out[m.Slug] = struct{}{}
	}
	return out, nil
}

// upsertModBase 写入单个 Mod 及其发布版本（幂等），返回前不处理依赖。
func upsertModBase(gormDB *gorm.DB, e modEntry) error {
	return gormDB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "slug"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"name_zh", "name_en", "aliases", "summary", "description", "categories", "loaders",
				"mc_versions", "homepage_url", "source_url", "license", "license_note", "game_edition",
				"data_version", "last_verified_at", "review_status", "reviewer", "reviewed_at",
				"review_note", "sort_order", "updated_at",
			}),
		}).Create(e.mod).Error; err != nil {
			return err
		}
		// 取回真实 id（冲突更新时 GORM 不一定回填）。
		var saved domain.Mod
		if err := tx.Where("slug = ?", e.mod.Slug).First(&saved).Error; err != nil {
			return err
		}

		// ---- 发布版本：按 (mod_id, version) upsert，并清理已删除的版本 ----
		versions := make([]string, 0, len(e.versions))
		for i := range e.versions {
			v := e.versions[i]
			v.ModID = saved.ID
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "mod_id"}, {Name: "version"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"release_type", "mc_versions", "loaders", "download_url", "download_note",
					"file_name", "published_at", "source_url", "verified", "verify_note",
					"sort_order", "updated_at",
				}),
			}).Create(&v).Error; err != nil {
				return err
			}
			versions = append(versions, v.Version)
		}
		cleanup := tx.Where("mod_id = ?", saved.ID)
		if len(versions) > 0 {
			cleanup = cleanup.Where("version NOT IN ?", versions)
		}
		return cleanup.Delete(&domain.ModVersion{}).Error
	})
}

// upsertModDeps 重建单个 Mod 的依赖 / 冲突（净效果幂等）。
// 需在全部 Mod 落库后调用，以便解析 target_mod_slug。
func upsertModDeps(gormDB *gorm.DB, e modEntry) error {
	return gormDB.Transaction(func(tx *gorm.DB) error {
		var saved domain.Mod
		if err := tx.Where("slug = ?", e.mod.Slug).First(&saved).Error; err != nil {
			return err
		}
		if err := tx.Where("mod_id = ?", saved.ID).Delete(&domain.ModDependency{}).Error; err != nil {
			return err
		}
		for _, d := range e.deps {
			dep := d.dep
			dep.ModID = saved.ID
			if d.slug != "" {
				var target domain.Mod
				if err := tx.Where("slug = ?", d.slug).First(&target).Error; err != nil {
					return err
				}
				id := target.ID
				dep.TargetModID = &id
				if dep.TargetModName == "" {
					dep.TargetModName = target.NameZh
				}
			}
			if err := tx.Create(&dep).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// upsertModpack 写入单个整合包及其 Mod 清单（幂等）。
func upsertModpack(gormDB *gorm.DB, e modpackEntry) error {
	return gormDB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "slug"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"name_zh", "name_en", "aliases", "summary", "description", "mc_version", "loader",
				"author", "homepage_url", "source_url", "license", "license_note", "data_version",
				"last_verified_at", "review_status", "reviewer", "reviewed_at", "review_note",
				"sort_order", "updated_at",
			}),
		}).Create(e.pack).Error; err != nil {
			return err
		}
		var saved domain.Modpack
		if err := tx.Where("slug = ?", e.pack.Slug).First(&saved).Error; err != nil {
			return err
		}

		// ---- Mod 清单：整体重建（净效果幂等）----
		if err := tx.Where("modpack_id = ?", saved.ID).Delete(&domain.ModpackMod{}).Error; err != nil {
			return err
		}
		for i := range e.mods {
			pm := e.mods[i]
			pm.ModpackID = saved.ID
			slug := e.slugs[i]
			if slug != "" {
				var target domain.Mod
				if err := tx.Where("slug = ?", slug).First(&target).Error; err != nil {
					return err
				}
				id := target.ID
				pm.ModID = &id
				if pm.ModName == "" {
					pm.ModName = target.NameZh
				}
			}
			if err := tx.Create(&pm).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// stringsOrEmpty 归一化字符串切片（nil → 空切片）。
func stringsOrEmpty(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
