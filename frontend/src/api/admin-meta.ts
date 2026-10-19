// ===== 分类 / 标签管理 API（全 POST + 动作子路径）=====
import http, { toPaged } from './http'
import type { ApiResponse } from '@/types/api'
import type { Category, CategoryQuery, Tag, TagQuery, TagUsageRow } from '@/types/category'

export interface CategoryPayload {
  name: string
  slug: string
  content_type: 'tool' | 'resource' | 'wiki'
  sort_order?: number
}

export interface TagPayload {
  name: string
  slug: string
}

// ===== 分类 =====
export async function listCategories(params: CategoryQuery = {}) {
  const { data } = await http.post<ApiResponse<Category[]>>('/admin/categories/list', params)
  return data.data
}

export async function createCategory(payload: CategoryPayload): Promise<Category> {
  const { data } = await http.post<ApiResponse<Category>>('/admin/categories/create', payload)
  return data.data
}

export async function updateCategory(id: string, payload: Partial<CategoryPayload>): Promise<Category> {
  const { data } = await http.post<ApiResponse<Category>>('/admin/categories/update', { id, ...payload })
  return data.data
}

export async function deleteCategory(id: string): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/categories/delete', { id })
}

// ===== 标签 =====
export async function listTags(params: TagQuery = {}) {
  const { data } = await http.post<ApiResponse<Tag[]>>('/admin/tags/list', params)
  return toPaged({ data })
}

export async function createTag(payload: TagPayload): Promise<Tag> {
  const { data } = await http.post<ApiResponse<Tag>>('/admin/tags/create', payload)
  return data.data
}

export async function updateTag(id: string, payload: Partial<TagPayload>): Promise<Tag> {
  const { data } = await http.post<ApiResponse<Tag>>('/admin/tags/update', { id, ...payload })
  return data.data
}

export async function deleteTag(id: string): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/tags/delete', { id })
}

/** 标签使用统计（跨资源 / 工具 / 百科，来自关联表聚合，按使用次数降序）。 */
export async function listTagUsage(limit = 200): Promise<TagUsageRow[]> {
  const { data } = await http.post<ApiResponse<TagUsageRow[]>>('/admin/tags/usage', { limit })
  return data.data
}