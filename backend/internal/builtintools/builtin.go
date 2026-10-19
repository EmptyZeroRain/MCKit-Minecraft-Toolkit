// Package builtintools 汇总内置工具（前端自带页面）的 slug 与落地路径。
//
// 单一事实源在前端 `frontend/src/utils/builtin-tools.ts`：新增内置工具时必须同步本包。
// 后端在此集中一份，供 sitemap 去重与使用统计（TODO §13）白名单校验复用，
// 避免多处硬编码清单漂移。
package builtintools

// Tool 内置工具的 slug 与落地路径。
type Tool struct {
	Slug  string
	Route string
}

// Tools 内置工具清单，顺序与前端一致。
var Tools = []Tool{
	{Slug: "recipes", Route: "/recipes"},
	{Slug: "coordinate", Route: "/coordinate"},
	{Slug: "items", Route: "/items"},
	{Slug: "potions", Route: "/potions"},
	{Slug: "villagers", Route: "/villagers"},
	{Slug: "mobs", Route: "/mobs"},
	{Slug: "redstone", Route: "/redstone"},
	{Slug: "mods", Route: "/mods"},
	{Slug: "dev-docs", Route: "/dev"},
	{Slug: "mappings", Route: "/mappings"},
	{Slug: "commands", Route: "/commands"},
	{Slug: "command-generator", Route: "/command-generator"},
	{Slug: "enchant-calculator", Route: "/enchant-calculator"},
	{Slug: "versions", Route: "/versions"},
	{Slug: "block-ids", Route: "/block-ids"},
	{Slug: "seed-map", Route: "/seed-map"},
	{Slug: "seed-tools", Route: "/seed-tools"},
	{Slug: "region-inspect", Route: "/region-inspect"},
}

// slugs 内置工具 slug 集合（由 Tools 派生）。
var slugs = func() map[string]bool {
	m := make(map[string]bool, len(Tools))
	for _, t := range Tools {
		m[t.Slug] = true
	}
	return m
}()

// IsBuiltin 判断 slug 是否为内置工具。
func IsBuiltin(slug string) bool { return slugs[slug] }

// Slugs 返回内置工具 slug 列表（拷贝，避免调用方改写内部状态）。
func Slugs() []string {
	out := make([]string, 0, len(Tools))
	for _, t := range Tools {
		out = append(out, t.Slug)
	}
	return out
}
