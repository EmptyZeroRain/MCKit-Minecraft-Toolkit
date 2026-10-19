// ===== 使用统计与运营看板类型（对齐后端 TODO §13 接口）=====

/**
 * 时间范围请求：
 *   - 只给 days 表示最近 N 天（含当天，默认 30，上限 366）；
 *   - 同时给出 from / to 时以 from / to 为准（YYYY-MM-DD，含首尾）。
 */
export interface AnalyticsRangeQuery {
  from?: string
  to?: string
  days?: number
  /** 明细列表条数上限（默认 50/100，上限 500） */
  limit?: number
}

/** 运营概览：全部为聚合计数与比率，不含任何个人身份信息 */
export interface UsageOverview {
  from: string
  to: string
  tool_visits: number
  tool_uses: number
  tool_failures: number
  /** 失败率 = failures / uses（无使用量时为 0） */
  tool_failure_rate: number
  active_tools: number
  resource_views: number
  active_resources: number
  /** 当前失效链接总数（实时查询，非时间范围内累加） */
  broken_links: number
  search_queries: number
  search_zero_result: number
  search_failed: number
  api_total: number
  api_errors: number
  api_client_errors: number
  api_rate_limited: number
  api_error_rate: number
  avg_latency_ms: number
  /** 由固定分桶近似的 P95（桶上界，保守值） */
  p95_latency_ms: number
}

/** 单个工具的使用统计 */
export interface ToolUsageStat {
  slug: string
  visits: number
  uses: number
  failures: number
  fail_rate: number
}

/** 单个资源的详情访问量 */
export interface ResourceUsageStat {
  slug: string
  views: number
}

/** 路由模板级 API 指标（route 为 chi 路由模板，不含具体参数值） */
export interface ApiUsageStat {
  route: string
  method: string
  total: number
  errors: number
  client_errors: number
  rate_limited: number
  avg_ms: number
  max_ms: number
  p95_ms: number
}

/** 某天的合并趋势点（区间内缺失日期后端补零） */
export interface UsageTrendPoint {
  date: string
  tool_visits: number
  tool_uses: number
  tool_failures: number
  resource_views: number
  search_queries: number
  api_total: number
  api_errors: number
  api_rate_limited: number
}
