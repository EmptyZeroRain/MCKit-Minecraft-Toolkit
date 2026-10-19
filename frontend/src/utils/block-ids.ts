// ===== 物品 ID 对照表筛选（原子能力，独立于视图便于单测）=====
// 移植自 CreeperCode 的本地过滤逻辑：数据量约千余条且静态，
// 一次全量加载后本地过滤，切换筛选零延迟、不重复打接口。
import type { BlockIdEntry } from '@/types/block-id'

/** kind 为 '' 或 'all' 表示不限分类 */
export type BlockIdKindFilter = string

/** 单条是否命中关键词：匹配命名空间 ID 或中文名，大小写不敏感 */
export function matchBlockId(entry: BlockIdEntry, keyword: string): boolean {
  const q = keyword.trim().toLowerCase()
  if (!q) return true
  return entry.id.toLowerCase().includes(q) || entry.name.toLowerCase().includes(q)
}

/** 按关键词 + 分类过滤 */
export function filterBlockIds(
  entries: BlockIdEntry[],
  keyword: string,
  kind: BlockIdKindFilter,
): BlockIdEntry[] {
  const noKindFilter = !kind || kind === 'all'
  return entries.filter((e) => {
    if (!noKindFilter && e.kind !== kind) return false
    return matchBlockId(e, keyword)
  })
}

/** 统计某分类条目数；'' 或 'all' 返回总数 */
export function countBlockIds(entries: BlockIdEntry[], kind: BlockIdKindFilter): number {
  if (!kind || kind === 'all') return entries.length
  return entries.filter((e) => e.kind === kind).length
}
