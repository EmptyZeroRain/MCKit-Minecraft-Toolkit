// 开发文档知识库导入（TODO §21「我的世界开发栏目」）。
//
// 数据文件为单个 JSON 对象：{ "dev_docs": [...] }。
// 校验策略（对应 §9「校验资料来源和版本适用范围」的同一原则）：
//   - slug / title_zh / loader / category / mc_versions 必填；
//   - loader 与 category 必须在白名单内；
//   - source_url 必填（可核验来源），official_url 至少有一个 http(s) 链接；
//     若两者不同，说明本站以官方仓库源文件作为可核验锚点，由 note 字段解释；
//   - api_entries 的 name 必填、kind 必须在白名单内；
//   - 版本声明沿用 versioning 校验，保证「自某版本起」语义合法。
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

// devDocSeedFile 开发文档资料文件结构。
type devDocSeedFile struct {
	DevDocs []devDocRow `json:"dev_docs"`
}

// devDocRow 开发文档导入原始行。
type devDocRow struct {
	Slug           string               `json:"slug"`
	TitleZh        string               `json:"title_zh"`
	TitleEn        string               `json:"title_en"`
	Loader         string               `json:"loader"`
	Category       string               `json:"category"`
	Summary        string               `json:"summary"`
	BodyMd         string               `json:"body_md"`
	APIEntries     []domain.DevApiEntry `json:"api_entries"`
	OfficialURL    string               `json:"official_url"`
	SourceURL      string               `json:"source_url"`
	MCVersions     []string             `json:"mc_versions"`
	Loaders        []string             `json:"loaders"`
	Tags           []string             `json:"tags"`
	SortOrder      int                  `json:"sort_order"`
	Status         string               `json:"status"`
	DataVersion    string               `json:"data_version"`
	LastVerifiedAt string               `json:"last_verified_at"`
}

// validDevApiKinds 接口条目类型白名单。
var validDevApiKinds = map[string]bool{
	"class":   true,
	"method":  true,
	"event":   true,
	"field":   true,
	"command": true,
	"file":    true,
	"plugin":  true,
}

// validDevStatuses 发布状态白名单。
var validDevStatuses = map[string]bool{"draft": true, "published": true}

// buildDevDoc 校验并归一化单行开发文档。
func buildDevDoc(row devDocRow) (*domain.DevDoc, error) {
	slug := strings.TrimSpace(row.Slug)
	titleZh := strings.TrimSpace(row.TitleZh)
	if slug == "" || titleZh == "" {
		return nil, errors.New("slug 与 title_zh 必填")
	}
	loader := strings.TrimSpace(row.Loader)
	if !domain.IsValidDevLoader(loader) {
		return nil, fmt.Errorf("loader 非法: %q（可选 %s）", row.Loader, strings.Join(domain.DevLoaders, " / "))
	}
	category := strings.TrimSpace(row.Category)
	if !domain.IsValidDevCategory(category) {
		return nil, fmt.Errorf("category 非法: %q（可选 %s）", row.Category, strings.Join(domain.DevCategories, " / "))
	}
	sourceURL := strings.TrimSpace(row.SourceURL)
	if sourceURL == "" {
		return nil, errors.New("source_url 必填（需明确资料来源）")
	}
	officialURL := strings.TrimSpace(row.OfficialURL)
	if officialURL == "" {
		officialURL = sourceURL
	}
	if !isHTTPURL(officialURL) || !isHTTPURL(sourceURL) {
		return nil, errors.New("official_url / source_url 必须是 http(s) 链接")
	}
	versions := row.MCVersions
	if len(versions) == 0 {
		return nil, errors.New("mc_versions 必填（需明确版本适用范围）")
	}
	if err := validateVersions("mc_versions", versions); err != nil {
		return nil, err
	}
	status := strings.TrimSpace(row.Status)
	if status == "" {
		status = "published"
	}
	if !validDevStatuses[status] {
		return nil, fmt.Errorf("status 非法: %q", row.Status)
	}

	// 平台集合：默认与 loader 一致；multi 条目应显式给出 loaders。
	loaders := make([]string, 0, len(row.Loaders))
	for _, l := range row.Loaders {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}
		if !domain.IsValidDevLoader(trimmed) {
			return nil, fmt.Errorf("loaders 含非法取值: %q", l)
		}
		loaders = append(loaders, trimmed)
	}
	if len(loaders) == 0 && loader != domain.DevLoaderMulti && loader != domain.DevLoaderTooling {
		loaders = append(loaders, loader)
	}

	apiEntries := make(domain.DevApiEntries, 0, len(row.APIEntries))
	for i, e := range row.APIEntries {
		name := strings.TrimSpace(e.Name)
		if name == "" {
			return nil, fmt.Errorf("api_entries[%d].name 必填", i)
		}
		kind := strings.TrimSpace(e.Kind)
		if kind == "" {
			kind = "class"
		}
		if !validDevApiKinds[kind] {
			return nil, fmt.Errorf("api_entries[%d].kind 非法: %q", i, e.Kind)
		}
		apiEntries = append(apiEntries, domain.DevApiEntry{
			Name:      name,
			Kind:      kind,
			Signature: strings.TrimSpace(e.Signature),
			Note:      strings.TrimSpace(e.Note),
		})
	}

	doc := &domain.DevDoc{
		Slug:        slug,
		TitleZh:     titleZh,
		TitleEn:     strings.TrimSpace(row.TitleEn),
		Loader:      loader,
		Category:    category,
		Summary:     strings.TrimSpace(row.Summary),
		BodyMd:      strings.TrimSpace(row.BodyMd),
		APIEntries:  apiEntries,
		OfficialURL: officialURL,
		SourceURL:   sourceURL,
		MCVersions:  types.JSONSlice(versioning.Sort(versions)),
		Loaders:     types.JSONSlice(loaders),
		Tags:        types.JSONSlice(devStringsOrEmpty(row.Tags)),
		SortOrder:   row.SortOrder,
		Status:      status,
		DataVersion: strings.TrimSpace(row.DataVersion),
	}
	if raw := strings.TrimSpace(row.LastVerifiedAt); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, fmt.Errorf("last_verified_at 需为 RFC3339 时间: %v", err)
		}
		doc.LastVerifiedAt = &t
	}
	return doc, nil
}

// importDevDocs 导入开发文档知识库。
func importDevDocs(gormDB *gorm.DB, content []byte, dryRun bool) *summary {
	var file devDocSeedFile
	if err := json.Unmarshal(content, &file); err != nil {
		fmt.Fprintf(os.Stderr, "JSON 解析失败: %v\n", err)
		os.Exit(1)
	}

	s := &summary{total: len(file.DevDocs)}
	seen := make(map[string]bool, len(file.DevDocs))

	for i, row := range file.DevDocs {
		doc, err := buildDevDoc(row)
		if err != nil {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("开发文档第 %d 行：%v", i+1, err))
			continue
		}
		if seen[doc.Slug] {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("开发文档 slug 重复: %s", doc.Slug))
			continue
		}
		seen[doc.Slug] = true
		if len(doc.APIEntries) == 0 {
			s.messages = append(s.messages, fmt.Sprintf("提示：%s 未列出关键接口", doc.Slug))
		}

		isNew, err := isNewRecord(gormDB, &domain.DevDoc{}, "slug = ?", doc.Slug)
		if err != nil {
			s.fail(fmt.Sprintf("%s 预查失败: %v", doc.Slug, err))
			continue
		}
		if dryRun {
			fmt.Printf("[dry-run] %s dev_doc %s (%s)\n", upsertVerb(isNew), doc.Slug, doc.TitleZh)
			s.countOutcome(isNew)
			continue
		}
		if err := gormDB.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "slug"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"title_zh", "title_en", "loader", "category", "summary", "body_md",
				"api_entries", "official_url", "source_url", "mc_versions", "loaders",
				"tags", "sort_order", "status", "data_version", "last_verified_at", "updated_at",
			}),
		}).Create(doc).Error; err != nil {
			s.fail(fmt.Sprintf("%s 导入失败: %v", row.Slug, err))
			continue
		}
		s.countOutcome(isNew)
	}
	return s
}

// isHTTPURL 判断是否为 http(s) 链接。
func isHTTPURL(raw string) bool {
	return strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")
}

// devStringsOrEmpty 归一化为非 nil 切片。
func devStringsOrEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
