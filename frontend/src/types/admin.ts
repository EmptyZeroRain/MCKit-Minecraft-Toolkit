// ===== 管理员 / 认证类型 =====
import type { AdminRole } from './common'

/** 管理端通用表格的列定义（配合 `components/admin/AdminTable.vue`）。 */
export interface AdminTableColumn {
  /** 列标识，同时用于 `cell-<key>` 插槽名 */
  key: string
  /** 表头文案 */
  label: string
  /** 对齐方式，默认左对齐 */
  align?: 'left' | 'center' | 'right'
  /** 可选列宽（如 `120px` / `20%`） */
  width?: string
}

export interface AdminUser {
  id: string
  /** 对外随机用户 ID（如 U7K3M2QX9F） */
  user_no?: string
  username: string
  /** 展示昵称；空串表示未设置，展示时回退到 username */
  nickname?: string
  /** 头像图片地址（http/https 外链）；空串表示未设置 */
  avatar_url?: string
  role: AdminRole
  /** 当前角色持有的权限点（由后端 /admin/auth/login 与 /me 返回，用于按权限显示后台入口） */
  permissions?: string[]
  /** 是否必须先修改密码（管理员重置密码后的首次登录） */
  must_change_password?: boolean
  is_active: boolean
  last_login_at: string | null
  created_at: string
}

/** 更新当前账号展示资料（用户信息页） */
export interface UpdateProfilePayload {
  nickname: string
  avatar_url: string
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
  /** 对外随机用户 ID（如 U7K3M2QX9F） */
  user_no: string
  username: string
  is_active: boolean
  last_login_at: string | null
  created_at: string
  favorite_count: number
  recent_count: number
  seed_count: number
}

/** 收藏明细 */
export interface PlatformUserFavorite {
  id: string
  kind: string
  object_id: string
  label: string
  url: string
  created_at: string
}

/** 最近使用明细 */
export interface PlatformUserRecent {
  id: string
  kind: string
  object_id: string
  label: string
  url: string
  visited_at: string
}

/** 常用种子明细 */
export interface PlatformUserSeed {
  id: string
  name: string
  seed: string
  note: string
  created_at: string
  updated_at: string
}

/** 普通用户详情（含个人数据明细） */
export interface PlatformUserDetail {
  id: string
  /** 对外随机用户 ID（如 U7K3M2QX9F） */
  user_no: string
  username: string
  is_active: boolean
  last_login_at: string | null
  created_at: string
  updated_at: string
  favorite_count: number
  recent_count: number
  seed_count: number
  favorites: PlatformUserFavorite[]
  recents: PlatformUserRecent[]
  seeds: PlatformUserSeed[]
}

/** 新建普通用户（角色固定为普通用户，前端不传角色） */
export interface CreateUserPayload {
  username: string
  password: string
}

/** 更新普通用户启用状态 */
export interface UpdateUserPayload {
  id: string
  is_active: boolean
}

/** 重置普通用户密码 */
export interface ResetUserPasswordPayload {
  id: string
  new_password: string
}

// ===== MC 源码缓存运维（需 ops.mcsrc 权限）=====

/** 单条源码缓存条目：jar + 索引 + 元数据的磁盘占用 */
export interface McsrcCacheEntry {
  version: string
  side: 'client' | 'server'
  cached: boolean
  /** jar 字节数 */
  size: number
  sha1: string
  indexed: boolean
  class_count: number
  index_size: number
  meta_size: number
  total_size: number
  fetched_at?: string | null
  index_built_at?: string | null
}

/** 源码缓存占用汇总 */
export interface McsrcCacheUsage {
  entries: number
  jar_bytes: number
  index_bytes: number
  meta_bytes: number
  total_bytes: number
}

/** 源码缓存列表响应 */
export interface McsrcCacheListResult {
  items: McsrcCacheEntry[]
  usage: McsrcCacheUsage
  /** 缓存占用告警阈值（字节）；0 表示未配置 */
  alert_threshold_bytes?: number
}

/** 缓存重建任务入队结果 */
export interface McsrcCacheRebuildResult {
  job_id: string
  type: string
  version: string
  side: string
}

/** 单版本清理结果 */
export interface McsrcCachePurgeResult {
  version: string
  side: string
  freed_bytes: number
}

/** 清空结果 */
export interface McsrcCachePurgeAllResult {
  purged: number
  freed_bytes: number
}