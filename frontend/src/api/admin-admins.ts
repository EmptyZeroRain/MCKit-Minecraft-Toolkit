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
