package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/repository/types"
	"mckit/backend/internal/service/resource"
)

// resourceRow 资源导入原始行（TODO §6 资源元数据批量导入）。
//
// Releases 为指针：字段缺省（null / 未出现）表示「不触碰已有发布版本」，
// 显式空数组表示「清空发布版本」。这样批量更新既能增量补齐，也能显式清空。
type resourceRow struct {
	Slug              string          `json:"slug"`
	Name              string          `json:"name"`
	Summary           string          `json:"summary"`
	Description       string          `json:"description"`
	ResourceType      string          `json:"resource_type"`
	CoverImageURL     string          `json:"cover_image_url"`
	AuthorName        string          `json:"author_name"`
	SourcePlatform    string          `json:"source_platform"`
	SourceURL         string          `json:"source_url"`
	License           string          `json:"license"`
	GameEdition       string          `json:"game_edition"`
	SupportedVersions []string        `json:"supported_versions"`
	LoaderTypes       []string        `json:"loader_types"`
	Dependencies      json.RawMessage `json:"dependencies"`
	InstallGuide      string          `json:"install_guide"`
	Tags              []string        `json:"tags"`
	CompatibilityNote string          `json:"compatibility_note"`
	Status            string          `json:"status"`
	Releases          *[]releaseRow   `json:"releases"`
}

// releaseRow 发布版本导入原始行（同一资源的不同发布版本）。
type releaseRow struct {
	VersionLabel string   `json:"version_label"`
	GameVersions []string `json:"game_versions"`
	LoaderTypes  []string `json:"loader_types"`
	SourceURL    string   `json:"source_url"`
	ReleasedAt   string   `json:"released_at"`
	Notes        string   `json:"notes"`
	IsLatest     bool     `json:"is_latest"`
}

// importResources 导入资源及其发布版本。
//
// 校验与规范化全部复用 internal/service/resource，保证导入与后台接口
// 共用同一套枚举与版本规则，避免两条写入路径产生不一致数据。
func importResources(gormDB *gorm.DB, content []byte, dryRun bool) *summary {
	var rows []resourceRow
	if err := json.Unmarshal(content, &rows); err != nil {
		fmt.Fprintf(os.Stderr, "JSON 解析失败: %v\n", err)
		os.Exit(1)
	}

	s := &summary{total: len(rows)}
	for i, row := range rows {
		label := strings.TrimSpace(row.Slug)
		if label == "" {
			label = fmt.Sprintf("第 %d 行", i+1)
		}

		res, releases, replaceReleases, err := buildResource(row)
		if err != nil {
			s.fail(fmt.Sprintf("%s：%v", label, err))
			continue
		}

		isNew, err := isNewRecord(gormDB, &domain.Resource{}, "slug = ?", res.Slug)
		if err != nil {
			s.fail(fmt.Sprintf("%s 预查失败: %v", res.Slug, err))
			continue
		}
		if dryRun {
			fmt.Printf("[dry-run] %s resource %s（发布版本 %d 个，替换=%v）\n",
				upsertVerb(isNew), res.Slug, len(releases), replaceReleases)
			s.countOutcome(isNew)
			continue
		}

		if err := upsertResource(gormDB, res, releases, replaceReleases); err != nil {
			s.fail(fmt.Sprintf("%s 导入失败: %v", res.Slug, err))
			continue
		}
		s.countOutcome(isNew)
	}
	return s
}

// buildResource 校验并构造资源实体与发布版本入参（纯函数，便于单测）。
func buildResource(row resourceRow) (*domain.Resource, []resource.ReleaseInput, bool, error) {
	slug := strings.TrimSpace(row.Slug)
	name := strings.TrimSpace(row.Name)
	summaryText := strings.TrimSpace(row.Summary)
	sourceURL := strings.TrimSpace(row.SourceURL)
	switch {
	case slug == "":
		return nil, nil, false, errors.New("slug 必填")
	case name == "":
		return nil, nil, false, errors.New("name 必填")
	case summaryText == "":
		return nil, nil, false, errors.New("summary 必填")
	case sourceURL == "":
		return nil, nil, false, errors.New("source_url 必填（资源原始来源）")
	}
	if !resource.ValidResourceType(row.ResourceType) {
		return nil, nil, false, fmt.Errorf("resource_type 非法: %s", row.ResourceType)
	}
	if !resource.ValidHTTPURL(sourceURL) {
		return nil, nil, false, errors.New("source_url 必须是 http/https 链接")
	}

	status := strings.TrimSpace(row.Status)
	if status == "" {
		status = domain.StatusDraft
	}
	if !resource.ValidResourceStatus(status) {
		return nil, nil, false, fmt.Errorf("status 非法: %s", status)
	}

	edition, err := resource.NormalizeGameEdition(row.GameEdition)
	if err != nil {
		return nil, nil, false, err
	}
	versions, err := resource.NormalizeGameVersions(row.SupportedVersions)
	if err != nil {
		return nil, nil, false, err
	}
	loaders, err := resource.NormalizeLoaderTypes(row.LoaderTypes)
	if err != nil {
		return nil, nil, false, err
	}
	deps, err := resource.NormalizeDependencies(row.Dependencies)
	if err != nil {
		return nil, nil, false, err
	}

	var (
		releases        []resource.ReleaseInput
		replaceReleases bool
	)
	if row.Releases != nil {
		replaceReleases = true
		releases = make([]resource.ReleaseInput, 0, len(*row.Releases))
		for _, r := range *row.Releases {
			releases = append(releases, resource.ReleaseInput{
				VersionLabel: r.VersionLabel,
				GameVersions: r.GameVersions,
				LoaderTypes:  r.LoaderTypes,
				SourceURL:    r.SourceURL,
				ReleasedAt:   r.ReleasedAt,
				Notes:        r.Notes,
				IsLatest:     r.IsLatest,
			})
		}
		if _, err := resource.NormalizeReleases(releases); err != nil {
			return nil, nil, false, err
		}
	}

	res := &domain.Resource{
		Slug:              slug,
		Name:              name,
		Summary:           summaryText,
		Description:       row.Description,
		ResourceType:      row.ResourceType,
		CoverImageURL:     row.CoverImageURL,
		AuthorName:        row.AuthorName,
		SourcePlatform:    row.SourcePlatform,
		SourceURL:         sourceURL,
		License:           row.License,
		GameEdition:       edition,
		SupportedVersions: types.JSONSlice(versions),
		LoaderTypes:       types.JSONSlice(loaders),
		Dependencies:      types.JSONB(deps),
		InstallGuide:      row.InstallGuide,
		Tags:              types.JSONSlice(row.Tags),
		CompatibilityNote: row.CompatibilityNote,
		Status:            status,
	}
	return res, releases, replaceReleases, nil
}

// upsertResource 以 slug 为业务主键幂等写入资源，并按需整体替换发布版本。
func upsertResource(gormDB *gorm.DB, res *domain.Resource, releases []resource.ReleaseInput, replaceReleases bool) error {
	if err := gormDB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "slug"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name", "summary", "description", "resource_type", "cover_image_url",
			"author_name", "source_platform", "source_url", "license", "game_edition",
			"supported_versions", "loader_types", "dependencies", "install_guide",
			"tags", "compatibility_note", "status", "updated_at",
		}),
	}).Create(res).Error; err != nil {
		return err
	}

	if !replaceReleases {
		return nil
	}

	// ON CONFLICT DO UPDATE 不保证回填主键，按 slug 重新取回资源 ID 更稳妥。
	var stored domain.Resource
	if err := gormDB.Select("id").Where("slug = ?", res.Slug).First(&stored).Error; err != nil {
		return err
	}
	svc := resource.NewService(gormDB)
	if _, err := svc.ReplaceReleases(context.Background(), stored.ID, releases); err != nil {
		return err
	}
	return nil
}
