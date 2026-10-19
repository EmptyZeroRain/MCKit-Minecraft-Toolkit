// ===== 管理端认证 API（需求 8.3，全 POST）=====
import http, { setCsrfToken } from './http'
import type { ApiResponse } from '@/types/api'
import type { AdminUser, ChangePasswordPayload, LoginPayload } from '@/types/admin'

/** 登录响应额外携带 CSRF token，用于写操作双重提交校验 */
type AdminAuthResponse = AdminUser & { csrf_token?: string }

/** 管理员登录（服务端校验凭证并种下会话 Cookie） */
export async function login(payload: LoginPayload): Promise<AdminUser> {
  const { data } = await http.post<ApiResponse<AdminAuthResponse>>('/admin/auth/login', payload)
  if (data.data.csrf_token) setCsrfToken(data.data.csrf_token)
  return data.data
}

/** 管理员退出（使会话失效） */
export async function logout(): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/auth/logout', {})
  setCsrfToken('')
}

/** 当前管理员信息 */
export async function me(): Promise<AdminUser> {
  const { data } = await http.post<ApiResponse<AdminAuthResponse>>('/admin/auth/me', {})
  if (data.data.csrf_token) setCsrfToken(data.data.csrf_token)
  return data.data
}

/** 修改密码 */
export async function changePassword(payload: ChangePasswordPayload): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/auth/change-password', payload)
}