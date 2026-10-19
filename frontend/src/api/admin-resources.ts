// ===== 资源管理 API（需求 8.4，全 POST + 动作子路径）=====
import http, { toPaged } from './http'
import type { ApiResponse } from '@/types/api'
import type {
  LinkCheckResult,
  Resource,
  ResourceDetail,
  ResourceListQuery,
  ResourcePayload,
  ResourceRelease,
  ResourceReleasePayload,
} from '@/types/resource'

/** 资源列表（分页 + 筛选） */
export async function listResources(params: ResourceListQuery = {}) {
  const { data } = await http.post<ApiResponse<Resource[]>>('/admin/resources/list', params)
  return toPaged({ data })
}

/** 资源详情（id，含发布版本与兼容性结论） */
export async function getResource(id: string): Promise<ResourceDetail | null> {
  const { data } = await http.post<ApiResponse<ResourceDetail>>('/admin/resources/detail', { id })
  return data.data
}

/** 创建资源 */
export async function createResource(payload: ResourcePayload): Promise<Resource> {
  const { data } = await http.post<ApiResponse<Resource>>('/admin/resources/create', payload)
  return data.data
}

/** 更新资源（白名单字段由后端校验） */
export async function updateResource(id: string, payload: Partial<ResourcePayload>): Promise<Resource> {
  const { data } = await http.post<ApiResponse<Resource>>('/admin/resources/update', { id, ...payload })
  return data.data
}

/** 删除 / 下架资源（软删除由后端实现） */
export async function deleteResource(id: string): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/resources/delete', { id })
}

/** 检查资源来源链接（防 SSRF 由后端执行） */
export async function checkResourceLink(id: string): Promise<LinkCheckResult> {
  const { data } = await http.post<ApiResponse<LinkCheckResult>>('/admin/resources/check-link', { id })
  return data.data
}

/** 发布版本列表（TODO §6：同一资源不同发布版本的关联） */
export async function listResourceReleases(id: string): Promise<ResourceRelease[]> {
  const { data } = await http.post<ApiResponse<ResourceRelease[]>>('/admin/resources/releases/list', { id })
  return data.data ?? []
}

/** 整体替换发布版本（以完整列表为准，幂等） */
export async function replaceResourceReleases(
  id: string,
  releases: ResourceReleasePayload[],
): Promise<ResourceRelease[]> {
  const { data } = await http.post<ApiResponse<ResourceRelease[]>>('/admin/resources/releases/replace', {
    id,
    releases,
  })
  return data.data ?? []
}