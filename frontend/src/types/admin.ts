// ===== 管理员 / 认证类型 =====
import type { AdminRole } from './common'

export interface AdminUser {
  id: string
  username: string
  role: AdminRole
  /** 当前角色持有的权限点（由后端 /admin/auth/login 与 /me 返回，用于按权限显示后台入口） */
  permissions?: string[]
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

// ===== 角色与权限（后台「角色管理」）=====

/** 权限点：key 为存储值，label 为中文名 */
export interface Permission {
  key: string
  label: string
}

/** 权限点分组（用于按组勾选） */
export interface PermissionGroup {
  key: string
  label: string
  permissions: Permission[]
}

/** 角色：name 为主键（稳定标识，创建后不可改） */
export interface RoleRow {
  name: string
  label: string
  description: string
  permissions: string[]
  is_system: boolean
  created_at: string
  updated_at: string
}

/** 新建角色（name 需以字母开头，仅含小写字母、数字、下划线、连字符） */
export interface CreateRolePayload {
  name: string
  label: string
  description?: string
  permissions: string[]
}

/** 更新角色（白名单字段，name 不可改） */
export interface UpdateRolePayload {
  name: string
  label?: string
  description?: string
  permissions?: string[]
}

// ===== 普通用户管理（平台账号，admin 专属）=====

/** 普通用户列表行：账号基础信息 + 个人数据计数 */
export interface PlatformUserRow {
  id: string
  username: string
  is_active: boolean
  last_login_at: string | null
  created_at: string
  favorite_count: number
  recent_count: number
  seed_count: number
}