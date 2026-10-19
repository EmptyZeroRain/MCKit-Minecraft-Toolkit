// 搜索词统计展示纯函数（可单测，不含副作用）
import type { SearchTermStat } from '@/types/search-term'

/**
 * 无结果占比（0~100 的整数百分比）。
 * hits 为 0 时按边界处理，避免出现 NaN / 超过 100。
 */
export function noResultRatio(t: Pick<SearchTermStat, 'hits' | 'no_result_hits'>): number {
  if (!t.hits || t.hits <= 0) return 0
  const ratio = Math.round((t.no_result_hits / t.hits) * 100)
  if (ratio < 0) return 0
  return ratio > 100 ? 100 : ratio
}

/** 是否为「高频无结果词」：无结果次数达到阈值（默认 3 次），说明存在资料缺口。 */
export function isHighNoResult(t: Pick<SearchTermStat, 'no_result_hits'>, threshold = 3): boolean {
  return t.no_result_hits >= threshold
}
