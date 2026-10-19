// ===== 我的世界开发栏目 API（TODO §21）=====
// 说明：按项目契约，所有接口统一使用 POST，查询参数放在请求体。
import http, { toPaged } from './http'
import type { ApiResponse } from '@/types/api'
import type { DevDocDetail, DevDocFacets, DevDocQuery, DevDocSummary } from '@/types/devdoc'
import type {
  MappingClassDetail,
  MappingSearchQuery,
  MappingSearchResponse,
  MappingSet,
  MappingSetQuery,
} from '@/types/mapping'

/** 开发文档列表与筛选（名称 / 平台 / 主题 / 游戏版本） */
export async function listDevDocs(params: DevDocQuery = {}) {
  const { data } = await http.post<ApiResponse<DevDocSummary[]>>('/dev-docs', params)
  return toPaged({ data })
}

/** 开发文档筛选维度统计（平台 / 主题计数，用于渲染筛选项） */
export async function getDevDocFacets(): Promise<DevDocFacets> {
  const { data } = await http.post<ApiResponse<DevDocFacets>>('/dev-docs/facets', {})
  return data.data
}

/** 开发文档详情（含同平台相关条目） */
export async function getDevDoc(slug: string): Promise<DevDocDetail> {
  const { data } = await http.post<ApiResponse<DevDocDetail>>(
    `/dev-docs/${encodeURIComponent(slug)}`,
    {},
  )
  return data.data
}

/** 已导入的映射数据集列表 */
export async function listMappingSets(params: MappingSetQuery = {}): Promise<MappingSet[]> {
  const { data } = await http.post<ApiResponse<{ sets: MappingSet[] }>>('/mappings/sets', params)
  return data.data?.sets ?? []
}

/** 映射查询：类 / 方法 / 字段 */
export async function searchMappings(params: MappingSearchQuery = {}): Promise<MappingSearchResponse> {
  const { data } = await http.post<ApiResponse<MappingSearchResponse>>('/mappings/search', params)
  return data.data
}

/** 类映射详情与成员列表 */
export async function getMappingClass(
  id: string,
  params: { kind?: string; q?: string; page?: number; page_size?: number } = {},
): Promise<MappingClassDetail> {
  const { data } = await http.post<ApiResponse<MappingClassDetail>>(
    `/mappings/class/${encodeURIComponent(id)}`,
    params,
  )
  return data.data
}
