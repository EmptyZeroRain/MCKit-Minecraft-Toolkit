// ===== 内容关键字屏蔽 API（ops.dataquality）=====
import http from './http'
import type { ApiResponse } from '@/types/api'
import type { ContentKeyword } from '@/types/content-filter'

/** 关键字列表（启用优先） */
export async function listContentKeywords(): Promise<ContentKeyword[]> {
  const { data } = await http.post<ApiResponse<{ items: ContentKeyword[]; total: number }>>(
    '/admin/content-health/keywords/list',
    {},
  )
  return data.data.items ?? []
}

/** 新增屏蔽关键字 */
export async function createContentKeyword(
  keyword: string,
  note = '',
): Promise<ContentKeyword> {
  const { data } = await http.post<ApiResponse<ContentKeyword>>(
    '/admin/content-health/keywords/create',
    { keyword, note },
  )
  return data.data
}

/** 更新关键字 / 备注 / 启用状态 */
export async function updateContentKeyword(input: {
  id: string
  keyword: string
  note: string
  enabled: boolean
}): Promise<ContentKeyword> {
  const { data } = await http.post<ApiResponse<ContentKeyword>>(
    '/admin/content-health/keywords/update',
    input,
  )
  return data.data
}

/** 删除关键字 */
export async function deleteContentKeyword(id: string): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/content-health/keywords/delete', { id })
}
