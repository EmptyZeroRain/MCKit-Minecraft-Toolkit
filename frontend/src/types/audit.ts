// ===== 审计日志实体类型 =====

export interface AuditLog {
  id: string
  actor_admin_id: string | null
  actor_username?: string
  action: string
  object_type: string
  object_id: string | null
  before_summary: Record<string, unknown> | null
  after_summary: Record<string, unknown> | null
  request_id: string | null
  source_ip: string | null
  result: 'success' | 'failure'
  created_at: string
}

export interface AuditLogQuery {
  action?: string
  object_type?: string
  result?: 'success' | 'failure' | ''
  source_ip?: string
  /** 只看账号安全事件（登录 / 改密 / MFA / 会话撤销） */
  security?: boolean
  page?: number
  page_size?: number
}