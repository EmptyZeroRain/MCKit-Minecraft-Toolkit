package resource

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/versioning"
)

// 依赖项与发布版本数量上限：防止单个资源塞入超大 JSON，影响读写与页面渲染。
const (
	maxDependencies = 50
	maxReleases     = 100
)

// Dependency 规范化后的依赖项（TODO §6）。
//
// 存库时统一补齐 type 字段，避免「依赖 / 前置 Mod / 已知冲突」的语义
// 由前端或展示层各自猜测；历史数据缺 type 时按 dependency 处理。
type Dependency struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	ResourceURL string `json:"resource_url,omitempty"`
	Required    bool   `json:"required"`
	Note        string `json:"note,omitempty"`
}

// NormalizeDependencies 校验并规范化依赖列表，输出可直接写入 JSONB 的规范 JSON。
//
// 规则：必须为数组；name 必填且 ≤160；type 缺省为 dependency 且必须合法；
// resource_url 可选但必须为 http(s)；note ≤300。
func NormalizeDependencies(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return json.RawMessage("[]"), nil
	}
	var items []Dependency
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, errors.New("dependencies 必须是数组")
	}
	if len(items) > maxDependencies {
		return nil, fmt.Errorf("dependencies 最多 %d 条", maxDependencies)
	}

	out := make([]Dependency, 0, len(items))
	for i, d := range items {
		d.Name = strings.TrimSpace(d.Name)
		if d.Name == "" {
			return nil, fmt.Errorf("dependencies[%d].name 必填", i)
		}
		if len(d.Name) > 160 {
			return nil, fmt.Errorf("dependencies[%d].name 过长（≤160）", i)
		}

		d.Type = strings.TrimSpace(d.Type)
		if d.Type == "" {
			d.Type = domain.DependencyTypeDependency
		}
		if !ValidDependencyType(d.Type) {
			return nil, fmt.Errorf("dependencies[%d].type 非法: %s", i, d.Type)
		}

		d.ResourceURL = strings.TrimSpace(d.ResourceURL)
		if d.ResourceURL != "" && !validHTTPURL(d.ResourceURL) {
			return nil, fmt.Errorf("dependencies[%d].resource_url 必须是 http/https 链接", i)
		}

		d.Note = strings.TrimSpace(d.Note)
		if len(d.Note) > 300 {
			return nil, fmt.Errorf("dependencies[%d].note 过长（≤300）", i)
		}
		out = append(out, d)
	}

	b, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// ParseDependencies 解析存库的依赖 JSON（用于展示与兼容性推导）。
// 脏数据按「无依赖」处理，不阻断资源读取。
func ParseDependencies(raw json.RawMessage) []Dependency {
	if len(raw) == 0 {
		return nil
	}
	var items []Dependency
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	for i := range items {
		if items[i].Type == "" {
			items[i].Type = domain.DependencyTypeDependency
		}
	}
	return items
}

// ValidDependencyType 判断依赖关系类型是否合法。
func ValidDependencyType(t string) bool {
	for _, v := range domain.DependencyTypes {
		if v == t {
			return true
		}
	}
	return false
}

// NormalizeLoaderTypes 校验并去重加载器列表（TODO §6 统一字段）。
func NormalizeLoaderTypes(loaders []string) ([]string, error) {
	out := make([]string, 0, len(loaders))
	seen := make(map[string]struct{}, len(loaders))
	for _, raw := range loaders {
		l := strings.TrimSpace(raw)
		if l == "" {
			continue
		}
		if !ValidLoader(l) {
			return nil, fmt.Errorf("loader_types 含非法加载器: %s", l)
		}
		if _, dup := seen[l]; dup {
			continue
		}
		seen[l] = struct{}{}
		out = append(out, l)
	}
	return out, nil
}

// ValidHTTPURL 判断是否为 http/https 链接（TODO §6，导入与接口共用）。
func ValidHTTPURL(u string) bool {
	u = strings.TrimSpace(u)
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

// ValidResourceType 判断资源类型是否合法（TODO §6 统一字段，导入与接口共用同一枚举）。
func ValidResourceType(t string) bool {
	for _, v := range domain.ResourceTypes {
		if v == t {
			return true
		}
	}
	return false
}

// ValidResourceStatus 判断资源状态是否可写入。
// removed 由软删除 / 下架接口管理，不允许通过普通写入直接设置。
func ValidResourceStatus(s string) bool {
	switch s {
	case domain.StatusDraft, domain.StatusPublished, domain.StatusHidden:
		return true
	}
	return false
}

// ValidLoader 判断加载器是否在允许枚举内。
func ValidLoader(l string) bool {
	for _, v := range domain.LoaderTypes {
		if v == l {
			return true
		}
	}
	return false
}

// NormalizeGameVersions 校验并规范化游戏版本声明（单版本 + 范围），按升序归一化。
// 复用 versioning 的解析与校验，保证与物品 / 配方 / 指令的版本规则一致。
func NormalizeGameVersions(versions []string) ([]string, error) {
	cleaned := make([]string, 0, len(versions))
	for _, v := range versions {
		if s := strings.TrimSpace(v); s != "" {
			cleaned = append(cleaned, s)
		}
	}
	if errs := versioning.Validate(cleaned); len(errs) > 0 {
		return nil, fmt.Errorf("supported_versions 非法：%v", errs[0])
	}
	return versioning.Sort(cleaned), nil
}

// NormalizeGameEdition 校验游戏版本类型（java / bedrock），空值默认 java。
func NormalizeGameEdition(edition string) (string, error) {
	e := strings.TrimSpace(edition)
	if e == "" {
		return domain.GameEditionJava, nil
	}
	if e != domain.GameEditionJava && e != domain.GameEditionBedrock {
		return "", fmt.Errorf("game_edition 非法: %s", e)
	}
	return e, nil
}

// ReleaseInput 单个发布版本的入参（创建 / 更新 / 导入共用）。
type ReleaseInput struct {
	VersionLabel string
	GameVersions []string
	LoaderTypes  []string
	SourceURL    string
	// ReleasedAt 发布时间，可为空（不猜测）；支持 RFC3339 或 YYYY-MM-DD。
	ReleasedAt string
	Notes      string
	IsLatest   bool
}

// NormalizeReleases 校验并规范化发布版本列表（TODO §6）。
//
// 规则：version_label 必填、≤120 且在同一资源内唯一；source_url 必填且为 http(s)；
// game_versions / loader_types 复用统一枚举与版本规则；is_latest 至多一条。
// 返回结果按「发布时间降序（空值排后）+ 版本标签」排序，保证展示稳定。
func NormalizeReleases(items []ReleaseInput) ([]ReleaseInput, error) {
	if len(items) > maxReleases {
		return nil, fmt.Errorf("releases 最多 %d 条", maxReleases)
	}

	out := make([]ReleaseInput, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	latestCount := 0
	for i, it := range items {
		it.VersionLabel = strings.TrimSpace(it.VersionLabel)
		if it.VersionLabel == "" {
			return nil, fmt.Errorf("releases[%d].version_label 必填", i)
		}
		if len(it.VersionLabel) > 120 {
			return nil, fmt.Errorf("releases[%d].version_label 过长（≤120）", i)
		}
		if _, dup := seen[it.VersionLabel]; dup {
			return nil, fmt.Errorf("releases[%d].version_label 重复：%s", i, it.VersionLabel)
		}
		seen[it.VersionLabel] = struct{}{}

		it.SourceURL = strings.TrimSpace(it.SourceURL)
		if it.SourceURL == "" {
			return nil, fmt.Errorf("releases[%d].source_url 必填", i)
		}
		if !validHTTPURL(it.SourceURL) {
			return nil, fmt.Errorf("releases[%d].source_url 必须是 http/https 链接", i)
		}

		versions, err := NormalizeGameVersions(it.GameVersions)
		if err != nil {
			return nil, fmt.Errorf("releases[%d]：%v", i, err)
		}
		it.GameVersions = versions

		loaders, err := NormalizeLoaderTypes(it.LoaderTypes)
		if err != nil {
			return nil, fmt.Errorf("releases[%d]：%v", i, err)
		}
		it.LoaderTypes = loaders

		it.ReleasedAt = strings.TrimSpace(it.ReleasedAt)
		if it.ReleasedAt != "" {
			if _, err := ParseReleaseDate(it.ReleasedAt); err != nil {
				return nil, fmt.Errorf("releases[%d].released_at 需为 RFC3339 或 YYYY-MM-DD", i)
			}
		}

		it.Notes = strings.TrimSpace(it.Notes)
		if len(it.Notes) > 500 {
			return nil, fmt.Errorf("releases[%d].notes 过长（≤500）", i)
		}
		if it.IsLatest {
			latestCount++
			if latestCount > 1 {
				return nil, errors.New("releases 只能有一条 is_latest")
			}
		}
		out = append(out, it)
	}

	sortReleases(out)
	return out, nil
}

// ParseReleaseDate 解析发布时间，接受 RFC3339 或 YYYY-MM-DD。
func ParseReleaseDate(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

// sortReleases 发布时间降序（空值排后），同时间按版本标签升序，保证展示稳定。
func sortReleases(items []ReleaseInput) {
	// 插入排序：列表规模小（≤100），且需保持「同标签」稳定性，无需引入排序库。
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && releaseLess(items[j], items[j-1]); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

// releaseLess 判断 a 是否应排在 b 之前。
func releaseLess(a, b ReleaseInput) bool {
	at, aok := releaseTime(a)
	bt, bok := releaseTime(b)
	switch {
	case aok && bok:
		if !at.Equal(bt) {
			return at.After(bt)
		}
	case aok != bok:
		// 有发布时间的排在「未知时间」之前。
		return aok
	}
	return a.VersionLabel < b.VersionLabel
}

func releaseTime(r ReleaseInput) (time.Time, bool) {
	if r.ReleasedAt == "" {
		return time.Time{}, false
	}
	t, err := ParseReleaseDate(r.ReleasedAt)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
