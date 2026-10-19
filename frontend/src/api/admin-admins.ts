// ===== 管理员账号管理 API（admin 专属，全 POST + 动作子路径）=====
import http, { toPaged } from './http'
import type { ApiResponse } from '@/types/api'
import type {
  AdminUser,
  CreateAdminPayload,
  ResetAdminPasswordPayload,
  UpdateAdminPayload,
} from '@/types/admin'

export interface AdminListQuery {
  q?: string
  page?: number
  page_size?: number
}

/** 管理员列表（分页） */
export async function listAdmins(params: AdminListQuery = {}) {
  const { data } = await http.post<ApiResponse<AdminUser[]>>('/admin/admins/list', params)
  return toPaged({ data })
}

/** 创建管理员账号 */
export async function createAdmin(payload: CreateAdminPayload): Promise<AdminUser> {
  const { data } = await http.post<ApiResponse<AdminUser>>('/admin/admins/create', payload)
  return data.data
}

/** 更新管理员角色/启用状态 */
export async function updateAdmin(payload: UpdateAdminPayload): Promise<AdminUser> {
  const { data } = await http.post<ApiResponse<AdminUser>>('/admin/admins/update', payload)
  return data.data
}

/** 重置管理员密码（成功后该账号全部会话失效） */
export async function resetAdminPassword(payload: ResetAdminPasswordPayload): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/admins/reset-password', payload)
}

/** 删除管理员账号 */
export async function deleteAdmin(id: string): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/admins/delete', { id })
}

/**
 * 解绑指定后台账号的双因子认证（账号恢复）。
 *
 * 适用场景：管理员更换手机且恢复码丢失。解绑后该账号登录只需密码，
 * 建议通知本人尽快重新绑定。
 */
export async function resetAdminMFA(id: string): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/admins/mfa/reset', { id })
}

/** 删除后台账号的影响预览（只读） */
export interface AdminDeletePreview {
  admin_id: string
  username: string
  role: string
  is_active: boolean
  is_self: boolean
  is_last_admin: boolean
  active_sessions: number
  reversible: boolean
  notes: string[]
}

/** 获取删除后台账号的影响预览 */
export async function deleteAdminPreview(id: string): Promise<AdminDeletePreview> {
  const { data } = await http.post<ApiResponse<AdminDeletePreview>>(
    '/admin/admins/delete-preview',
    { id },
  )
  return data.data
}
