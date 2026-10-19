// ===== 普通用户管理 API（admin 专属，全 POST + 动作子路径）=====
import http, { toPaged } from './http'
import type { ApiResponse } from '@/types/api'
import type { PlatformUserRow } from '@/types/admin'

export interface UserListQuery {
  q?: string
  /** 省略表示不限；active 仅启用，disabled 仅禁用 */
  status?: '' | 'active' | 'disabled'
  page?: number
  page_size?: number
}

/** 普通用户列表（分页；每条含收藏 / 最近使用 / 常用种子计数） */
export async function listUsers(params: UserListQuery = {}) {
  const { data } = await http.post<ApiResponse<PlatformUserRow[]>>('/admin/users/list', params)
  return toPaged({ data })
}
