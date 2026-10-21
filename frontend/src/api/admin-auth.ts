// ===== 管理端认证 API（需求 8.3，全 POST）=====
import http, { setCsrfToken } from './http'
import type { ApiResponse } from '@/types/api'
import type { AdminUser, ChangePasswordPayload, LoginPayload, UpdateProfilePayload } from '@/types/admin'

/** 登录响应额外携带 CSRF token，用于写操作双重提交校验 */
type AdminAuthResponse = AdminUser & { csrf_token?: string }

/**
 * 登录结果：
 *   - 未启用 MFA：直接返回登录态用户；
 *   - 已启用 MFA：只返回 mfa_required + 短期票据，需再提交一次验证码（/admin/auth/mfa/verify）。
 * 此时服务端**不会**种下会话 Cookie，前端不应把用户视为已登录。
 */
export type LoginOutcome =
  | { mfa_required: true; mfa_ticket: string }
  | { mfa_required?: false; user: AdminUser }

/** 管理员登录（服务端校验凭证并种下会话 Cookie） */
export async function login(payload: LoginPayload): Promise<LoginOutcome> {
  const { data } = await http.post<ApiResponse<AdminAuthResponse & { mfa_required?: boolean; mfa_ticket?: string }>>(
    '/admin/auth/login',
    payload,
  )
  if (data.data.mfa_required) {
    return { mfa_required: true, mfa_ticket: data.data.mfa_ticket ?? '' }
  }
  if (data.data.csrf_token) setCsrfToken(data.data.csrf_token)
  return { user: data.data }
}

/** 第二因子校验（6 位 TOTP 或一次性恢复码），成功后种下会话 Cookie */
export async function verifyMFA(ticket: string, code: string): Promise<AdminUser> {
  const { data } = await http.post<ApiResponse<AdminAuthResponse>>('/admin/auth/mfa/verify', {
    ticket,
    code,
  })
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

/** 更新当前账号展示资料（昵称 / 头像），返回更新后的账号 */
export async function updateProfile(payload: UpdateProfilePayload): Promise<AdminUser> {
  const { data } = await http.post<ApiResponse<AdminUser>>('/admin/auth/profile', payload)
  return data.data
}

/** 会话元数据（不含会话 ID 原文，仅返回不可逆指纹） */
export interface AdminSessionItem {
  id: string
  role: string
  created_at: string
  last_seen_at: string
  expires_at: string
  current: boolean
}

/** 列出当前后台账号的全部有效会话 */
export async function listSessions(): Promise<AdminSessionItem[]> {
  const { data } = await http.post<ApiResponse<{ sessions: AdminSessionItem[]; total: number }>>(
    '/admin/auth/sessions/list',
    {},
  )
  return data.data.sessions ?? []
}

/** 撤销当前后台账号的会话：others 保留当前设备，all 含当前设备 */
export async function revokeSessions(
  scope: 'others' | 'all',
): Promise<{ scope: string; revoked: number }> {
  const { data } = await http.post<ApiResponse<{ scope: string; revoked: number }>>(
    '/admin/auth/sessions/revoke',
    { scope },
  )
  return data.data
}

/**
 * 登录历史条目。
 *
 * 与「会话列表」的区别：会话是当前有效连接，登录历史是发生过的登录事件
 * （含失败尝试），因此包含设备与来源 IP，且不随会话过期消失。
 */
export interface LoginHistoryItem {
  created_at: string
  source_ip: string
  /** 登录时的原始 User-Agent，历史数据可能为空 */
  user_agent: string
  result: 'success' | 'failure'
  action: string
}

/** 登录历史分页结果（默认第 1 页、每页 10 条） */
export interface LoginHistoryPage {
  items: LoginHistoryItem[]
  total: number
  page: number
  page_size: number
}

/** 分页列出当前后台账号的登录记录（成功与失败，按时间倒序） */
export async function listLoginHistory(page = 1, pageSize = 10): Promise<LoginHistoryPage> {
  const { data } = await http.post<ApiResponse<LoginHistoryPage>>('/admin/auth/login-history', {
    page,
    page_size: pageSize,
  })
  return data.data
}

// ===== 双因子认证（MFA）=====

/** 当前账号的 MFA 状态（不含任何密钥材料） */
export interface MFAStatus {
  enabled: boolean
  pending: boolean
  recovery_codes_left: number
  last_used_at: string | null
  /** 服务端是否已配置 MFA 加密密钥；false 时不可绑定 */
  configured: boolean
}

/** 绑定初始化结果：secret 供手动输入，uri 供渲染二维码 */
export interface MFASetupResult {
  secret: string
  uri: string
}

/** 查询当前账号 MFA 状态 */
export async function mfaStatus(): Promise<MFAStatus> {
  const { data } = await http.post<ApiResponse<MFAStatus>>('/admin/auth/mfa/status', {})
  return data.data
}

/** 生成新的 TOTP 绑定（待启用），返回 secret 与 otpauth URI */
export async function mfaSetup(): Promise<MFASetupResult> {
  const { data } = await http.post<ApiResponse<MFASetupResult>>('/admin/auth/mfa/setup', {})
  return data.data
}

/** 用首次验证码确认绑定，返回一次性恢复码（仅此一次可见） */
export async function mfaEnable(code: string): Promise<string[]> {
  const { data } = await http.post<ApiResponse<{ recovery_codes: string[] }>>(
    '/admin/auth/mfa/enable',
    { code },
  )
  return data.data.recovery_codes ?? []
}

/** 解绑 MFA（需密码 + 当前验证码双重证明） */
export async function mfaDisable(password: string, code: string): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/auth/mfa/disable', { password, code })
}