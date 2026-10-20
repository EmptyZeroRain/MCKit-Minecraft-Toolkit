// ===== 全站会话管理 API（需 ops.sessions）=====
// 契约来源：backend/internal/http/handler/admin_sessions.go
//
// 后端只返回会话展示指纹，不回显 SessionID 原文；踢下线时提交该指纹。
import http, { toPaged } from './http'
import type { ApiResponse, ListResult } from '@/types/api'

export type AdminSessionAccountType = 'user' | 'admin'

export interface AdminSessionItem {
  /** 会话展示指纹（非 SessionID 原文） */
  id: string
  username: string
  role: string
  account_type: AdminSessionAccountType
  created_at: string
  last_seen_at: string
  expires_at: string
  current: boolean
}

export interface AdminSessionListQuery {
  q?: string
  account_type?: 'all' | AdminSessionAccountType
  page?: number
  page_size?: number
}

/** 全站有效会话（普通用户 + 管理员） */
export async function listAdminSessions(
  params: AdminSessionListQuery = {},
): Promise<ListResult<AdminSessionItem>> {
  const { data } = await http.post<ApiResponse<AdminSessionItem[]>>('/admin/sessions/list', params)
  return toPaged({ data })
}

/** 按展示指纹踢下线单个会话 */
export async function revokeAdminSession(id: string): Promise<void> {
  await http.post<ApiResponse<{ revoked: boolean }>>('/admin/sessions/revoke', { id })
}
