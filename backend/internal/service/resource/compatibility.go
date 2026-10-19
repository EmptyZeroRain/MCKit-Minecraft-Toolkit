package resource

import (
	"mckit/backend/internal/domain"
	"mckit/backend/internal/versioning"
)

// Compatibility 兼容性结论（TODO §6）。
//
// 设计原则：**只有在存在可靠数据依据时才给出结论**。
// 资源级字段（supported_versions / loader_types）与发布版本均可作为依据；
// 两者都为空时 Verified=false，并附带未验证提示，由前端明确展示「未验证」，
// 绝不把「没有数据」当成「兼容」。
type Compatibility struct {
	Verified     bool     `json:"verified"`
	GameVersions []string `json:"game_versions"`
	LoaderTypes  []string `json:"loader_types"`
	// Sources 结论依据来源：resource（资源级字段）/ releases（发布版本）。
	Sources []string `json:"sources"`
	// AllVersions 是否声明了「所有版本」通配（game_versions 中出现 all）。
	// 前端据此展示「全部 MC 版本」并按官方版本清单展开完整列表。
	AllVersions bool `json:"all_versions"`
	// Note 人工核验备注（resources.compatibility_note）。
	Note string `json:"note"`
	// UnverifiedHint 未验证时的说明文案（仅 Verified=false 时返回）。
	UnverifiedHint string `json:"unverified_hint,omitempty"`
}

// unverifiedHint 未验证时的统一说明，明确「不代表可用」。
const unverifiedHint = "该资源未提供适用版本或加载器信息，兼容性尚未验证。"

// BuildCompatibility 依据资源级字段与发布版本推导兼容性结论。
func BuildCompatibility(res *domain.Resource, releases []domain.ResourceRelease) Compatibility {
	c := Compatibility{
		// 预置空切片：保证 JSON 输出稳定为 []，前端无需区分 null / []。
		GameVersions: []string{},
		LoaderTypes:  []string{},
		Sources:      []string{},
		Note:         res.CompatibilityNote,
	}

	versions := make([]string, 0, len(res.SupportedVersions))
	loaders := make([]string, 0, len(res.LoaderTypes))
	if len(res.SupportedVersions) > 0 || len(res.LoaderTypes) > 0 {
		c.Sources = append(c.Sources, "resource")
		versions = append(versions, res.SupportedVersions...)
		loaders = append(loaders, res.LoaderTypes...)
	}

	releaseHasData := false
	for _, r := range releases {
		if len(r.GameVersions) > 0 || len(r.LoaderTypes) > 0 {
			releaseHasData = true
		}
		versions = append(versions, r.GameVersions...)
		loaders = append(loaders, r.LoaderTypes...)
	}
	if releaseHasData {
		c.Sources = append(c.Sources, "releases")
	}

	c.GameVersions = versioning.Sort(dedupeStrings(versions))
	c.LoaderTypes = sortLoaders(dedupeStrings(loaders))
	c.AllVersions = versioning.IsAll(c.GameVersions)
	c.Verified = len(c.GameVersions) > 0 || len(c.LoaderTypes) > 0
	if !c.Verified {
		c.UnverifiedHint = unverifiedHint
	}
	return c
}

// dedupeStrings 去重并丢弃空串，保持首次出现顺序。
func dedupeStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, v := range in {
		if v == "" {
			continue
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// sortLoaders 按枚举定义顺序排序，保证展示稳定（未知值排在末尾并保持相对顺序）。
// 注意：不能用 append([]string(nil), in...) —— 零元素时返回 nil，
// 会让 JSON 序列化出 null 而非 []，破坏接口契约。
func sortLoaders(in []string) []string {
	rank := make(map[string]int, len(domain.LoaderTypes))
	for i, l := range domain.LoaderTypes {
		rank[l] = i
	}
	out := make([]string, len(in))
	copy(out, in)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && loaderLess(out[j], out[j-1], rank); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func loaderLess(a, b string, rank map[string]int) bool {
	ra, aok := rank[a]
	rb, bok := rank[b]
	switch {
	case aok && bok:
		return ra < rb
	case aok != bok:
		return aok
	}
	return false
}
