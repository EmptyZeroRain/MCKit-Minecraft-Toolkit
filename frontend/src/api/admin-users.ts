// ===== 普通用户管理 API（需 accounts.users 权限，全 POST + 动作子路径）=====
import http, { toPaged } from './http'
import type { ApiResponse } from '@/types/api'
import type {
  CreateUserPayload,
  PlatformUserDetail,
  PlatformUserRow,
  ResetUserPasswordPayload,
  UpdateUserPayload,
} from '@/types/admin'

export interface UserListQuery {
  q?: string
  /** 省略表示不限；active 仅启用，disabled 仅禁用 */
  status?: '' | 'active' | 'disabled'
  page?: number
  page_size?: number
}

/** 导出筛选条件（与列表一致，但不分页） */
export interface UserExportQuery {
  q?: string
  status?: '' | 'active' | 'disabled'
}

/** 普通用户列表（分页；每条含收藏 / 最近使用 / 常用种子计数） */
export async function listUsers(params: UserListQuery = {}) {
  const { data } = await http.post<ApiResponse<PlatformUserRow[]>>('/admin/users/list', params)
  return toPaged({ data })
}

/** 新建普通用户（角色固定为普通用户） */
export async function createUser(payload: CreateUserPayload): Promise<PlatformUserRow> {
  const { data } = await http.post<ApiResponse<PlatformUserRow>>('/admin/users/create', payload)
  return data.data
}

/** 普通用户详情（含收藏 / 最近使用 / 常用种子明细） */
export async function getUserDetail(id: string): Promise<PlatformUserDetail> {
  const { data } = await http.post<ApiResponse<PlatformUserDetail>>('/admin/users/detail', { id })
  return data.data
}

/** 启用 / 禁用普通用户（停用后该用户会话立即失效） */
export async function updateUser(payload: UpdateUserPayload): Promise<PlatformUserRow> {
  const { data } = await http.post<ApiResponse<PlatformUserRow>>('/admin/users/update', payload)
  return data.data
}

/** 重置普通用户密码（成功后该用户全部会话失效） */
export async function resetUserPassword(payload: ResetUserPasswordPayload): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/users/reset-password', payload)
}

/** 删除普通用户（个人数据由后端级联清除） */
export async function deleteUser(id: string): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/users/delete', { id })
}

/** 删除普通用户的影响预览（只读）：将一并清理的关联数据范围 */
export interface UserDeletePreview {
  user_id: string
  username: string
  is_active: boolean
  favorites: number
  recents: number
  seeds: number
  submissions: number
  sessions: number
  reversible: boolean
  notes: string[]
}

/** 获取删除普通用户的影响预览 */
export async function deleteUserPreview(id: string): Promise<UserDeletePreview> {
  const { data } = await http.post<ApiResponse<UserDeletePreview>>(
    '/admin/users/delete-preview',
    { id },
  )
  return data.data
}

/** 导出普通用户 CSV；返回文件内容与是否被截断 */
export async function exportUsersCsv(
  params: UserExportQuery = {},
): Promise<{ blob: Blob; truncated: boolean }> {
  const res = await http.post('/admin/users/export', params, { responseType: 'blob' })
  return {
    blob: res.data as Blob,
    truncated: String(res.headers['x-export-truncated'] ?? '') === 'true',
  }
}
