// ===== 百科管理 API（需求 8.4，全 POST + 动作子路径）=====
import http, { toPaged } from './http'
import type { ApiResponse } from '@/types/api'
import type { WikiEntry, WikiListQuery, WikiPayload } from '@/types/wiki'

export async function listWiki(params: WikiListQuery = {}) {
  const { data } = await http.post<ApiResponse<WikiEntry[]>>('/admin/wiki/list', params)
  return toPaged({ data })
}

export async function getWiki(id: string): Promise<WikiEntry | null> {
  const { data } = await http.post<ApiResponse<WikiEntry>>('/admin/wiki/detail', { id })
  return data.data
}

export async function createWiki(payload: WikiPayload): Promise<WikiEntry> {
  const { data } = await http.post<ApiResponse<WikiEntry>>('/admin/wiki/create', payload)
  return data.data
}

export async function updateWiki(id: string, payload: Partial<WikiPayload>): Promise<WikiEntry> {
  const { data } = await http.post<ApiResponse<WikiEntry>>('/admin/wiki/update', { id, ...payload })
  return data.data
}

export async function deleteWiki(id: string): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/wiki/delete', { id })
}