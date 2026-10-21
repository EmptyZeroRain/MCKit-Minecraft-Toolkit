// ===== 统一账号管理 API（后台「账号管理」单列表）=====
// 一个列表同时管理普通用户与后台账号；后端按目标账号角色做权限点二次判定。
import http, { toPaged } from './http'
import type { ApiResponse } from '@/types/api'

/** 列表范围：空串表示全部（仅 accounts.admins 可用）。 */
export type AccountScope = '' | 'admin' | 'user'

/** 账号列表条目（不含密码信息）。 */
export interface AccountRow {
  id: string
  /** 对外随机用户 ID（如 U7K3M2QX9F） */
  user_no: string
  username: string
  nickname: string
  email: string
  /** 角色标识：内置 user 为普通用户，其余为后台角色。 */
  role: string
  is_active: boolean
  last_login_at: string | null
  created_at: string
  /** 礼物币余额（无钱包记录为 0） */
  gift_coins: number
  /** 有效成长等级（后台覆盖值优先） */
  level: number
}

/** 收藏明细 */
export interface AccountFavorite {
  id: string
  kind: string
  object_id: string
  label: string
  url: string
  created_at: string
}

/** 最近使用明细 */
export interface AccountRecent {
  id: string
  kind: string
  object_id: string
  label: string
  url: string
  visited_at: string
}

/** 常用种子明细 */
export interface AccountSeed {
  id: string
  name: string
  seed: string
  note: string
  created_at: string
  updated_at: string
}

/** 账号详情（含个人数据明细）。 */
export interface AccountDetail {
  id: string
  user_no: string
  username: string
  nickname: string
  email: string
  avatar_url: string
  role: string
  is_active: boolean
  last_login_at: string | null
  created_at: string
  updated_at: string
  favorite_count: number
  recent_count: number
  seed_count: number
  favorites: AccountFavorite[]
  recents: AccountRecent[]
  seeds: AccountSeed[]
}

export interface AccountListQuery {
  q?: string
  /** 省略表示不限；active 仅启用，disabled 仅禁用 */
  status?: '' | 'active' | 'disabled'
  /** 列表范围；仅持有 accounts.users 时后端强制为 user */
  scope?: AccountScope
  page?: number
  page_size?: number
}

/** 导出筛选条件（与列表一致，但不分页）。 */
export type AccountExportQuery = Omit<AccountListQuery, 'page' | 'page_size'>

export interface CreateAccountPayload {
  username: string
  password: string
  /** 省略时后端按普通用户创建。 */
  role: string
}

export interface UpdateAccountPayload {
  id: string
  /** 目标角色；不传表示不改角色。 */
  role?: string
  is_active?: boolean
}

export interface ResetAccountPasswordPayload {
  id: string
  new_password: string
}

/** 删除影响预览。 */
export interface AccountDeletePreview {
  account_id: string
  username: string
  role: string
  is_active: boolean
  is_self: boolean
  is_last_admin: boolean
  favorites: number
  recents: number
  seeds: number
  submissions: number
  active_sessions: number
  reversible: boolean
  notes: string[]
}

/** 统一账号列表（分页）。 */
export async function listAccounts(params: AccountListQuery = {}) {
  const { data } = await http.post<ApiResponse<AccountRow[]>>('/admin/accounts/list', params)
  return toPaged({ data })
}

/** 创建账号（普通用户或后台角色）。 */
export async function createAccount(payload: CreateAccountPayload): Promise<AccountRow> {
  const { data } = await http.post<ApiResponse<AccountRow>>('/admin/accounts/create', payload)
  return data.data
}

/** 账号详情（含收藏 / 最近使用 / 常用种子明细）。 */
export async function getAccountDetail(id: string): Promise<AccountDetail> {
  const { data } = await http.post<ApiResponse<AccountDetail>>('/admin/accounts/detail', { id })
  return data.data
}

/** 更新角色 / 启用状态（角色变化后该账号会话立即失效）。 */
export async function updateAccount(payload: UpdateAccountPayload): Promise<AccountRow> {
  const { data } = await http.post<ApiResponse<AccountRow>>('/admin/accounts/update', payload)
  return data.data
}

/** 重置密码（成功后该账号会话全部失效）。 */
export async function resetAccountPassword(payload: ResetAccountPasswordPayload): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/accounts/reset-password', payload)
}

/** 删除账号（个人数据由后端级联清除）。 */
export async function deleteAccount(id: string): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/accounts/delete', { id })
}

/** 删除账号的影响预览（只读）。 */
export async function deleteAccountPreview(id: string): Promise<AccountDeletePreview> {
  const { data } = await http.post<ApiResponse<AccountDeletePreview>>('/admin/accounts/delete-preview', {
    id,
  })
  return data.data
}

/**
 * 解绑指定账号的双因子认证（账号恢复）。
 *
 * 需要 accounts.admins；账号更换手机且恢复码丢失时使用。
 */
export async function resetAccountMFA(id: string): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/accounts/mfa/reset', { id })
}

/** 导出账号 CSV；返回文件内容与是否被截断。 */
export async function exportAccountsCsv(
  params: AccountExportQuery = {},
): Promise<{ blob: Blob; truncated: boolean }> {
  const res = await http.post('/admin/accounts/export', params, { responseType: 'blob' })
  return {
    blob: res.data as Blob,
    truncated: String(res.headers['x-export-truncated'] ?? '') === 'true',
  }
}
