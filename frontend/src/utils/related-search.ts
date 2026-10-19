// 跨模块检索入口（TODO §3 跨模块资料关联）
//
// 设计原则：只有在存在可靠元数据时才给出直链；其它模块以「按名称检索」入口呈现，
// 由用户自行确认，绝不伪造关联或链接到未发布内容。
import type { SearchResultItem } from '@/types/search'

/** 全站搜索支持的类型（与后端 search.Filter.Type 一致）。 */
export type SearchEntryType = SearchResultItem['type']

/** 生成全站搜索入口（按名称 + 类型）。 */
export function searchEntry(name: string, type: SearchEntryType): string {
  return `/search?q=${encodeURIComponent(name)}&type=${type}`
}

/**
 * 生成指令模块检索入口。
 * 指令不在全站搜索类型内（全站搜索仅覆盖工具/资源/百科/物品），
 * 因此指向指令页自身的检索参数。
 */
export function commandSearchEntry(name: string): string {
  return `/commands?q=${encodeURIComponent(name)}`
}
