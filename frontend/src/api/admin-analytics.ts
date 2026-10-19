// ===== 运营看板 API（全 POST + 动作子路径，admin 专属；TODO §13）=====
// 只读接口；返回均为按天聚合的计数与比率，不含任何个人身份信息。
import http from './http'
import type { ApiResponse } from '@/types/api'
import type {
  AnalyticsRangeQuery,
  ApiUsageStat,
  ResourceUsageStat,
  ToolUsageStat,
  UsageOverview,
  UsageTrendPoint,
} from '@/types/analytics'

/** 运营概览（工具 / 资源 / 搜索 / API 汇总） */
export async function fetchUsageOverview(params: AnalyticsRangeQuery = {}) {
  const { data } = await http.post<ApiResponse<UsageOverview>>('/admin/analytics/overview', params)
  return data.data
}

/** 工具使用明细（按访问 + 使用总量降序） */
export async function fetchToolUsage(params: AnalyticsRangeQuery = {}) {
  const { data } = await http.post<ApiResponse<ToolUsageStat[]>>('/admin/analytics/tools', params)
  return data.data
}

/** 资源详情访问明细（按访问量降序） */
export async function fetchResourceUsage(params: AnalyticsRangeQuery = {}) {
  const { data } = await http.post<ApiResponse<ResourceUsageStat[]>>('/admin/analytics/resources', params)
  return data.data
}

/** 关键 API 指标（路由模板级，含近似 P95） */
export async function fetchApiUsage(params: AnalyticsRangeQuery = {}) {
  const { data } = await http.post<ApiResponse<ApiUsageStat[]>>('/admin/analytics/api', params)
  return data.data
}

/** 按天趋势（区间内缺失日期补零） */
export async function fetchUsageTrend(params: AnalyticsRangeQuery = {}) {
  const { data } = await http.post<ApiResponse<UsageTrendPoint[]>>('/admin/analytics/trend', params)
  return data.data
}
