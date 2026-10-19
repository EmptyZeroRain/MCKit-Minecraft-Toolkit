// ===== 管理员 / 认证类型 =====
import type { AdminRole } from './common'

export interface AdminUser {
  id: string
  username: string
  role: AdminRole
  is_active: boolean
  last_login_at: string | null
  created_at: string
}

export interface LoginPayload {
  username: string
  password: string
}

export interface ChangePasswordPayload {
  old_password: string
  new_password: string
}

/** 创建管理员账号（admin 专属，密码 ≥12 位） */
export interface CreateAdminPayload {
  username: string
  password: string
  role: AdminRole
}

/** 更新管理员角色/启用状态（白名单字段） */
export interface UpdateAdminPayload {
  id: string
  role?: AdminRole
  is_active?: boolean
}

/** 重置管理员密码（admin 专属） */
export interface ResetAdminPasswordPayload {
  id: string
  new_password: string
}