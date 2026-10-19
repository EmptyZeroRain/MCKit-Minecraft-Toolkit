// 搜索词统计类型（TODO §5 搜索体验）
//
// 统计内容为匿名化聚合结果：只含归一化关键词与计数，不含任何用户标识。

/** 单个搜索词的聚合统计 */
export interface SearchTermStat {
  id: string
  /** 归一化后的关键词（小写、折叠空白） */
  term: string
  /** 被搜索总次数 */
  hits: number
  /** 其中无结果的次数 */
  no_result_hits: number
  /** 查询本身失败的次数（超时 / 服务错误），与无结果区分 */
  failed_hits: number
  /** 最近一次搜索的结果数（0 表示无结果） */
  last_result_count: number
  first_seen_at: string
  last_seen_at: string
}

/** 统计视图：热门搜索 / 高频无结果词 / 查询失败词 */
export type SearchTermKind = 'hot' | 'no_result' | 'failed'

/** 搜索词统计查询参数 */
export interface SearchTermQuery {
  kind?: SearchTermKind
  /** 关键词模糊过滤 */
  q?: string
  page?: number
  page_size?: number
}
