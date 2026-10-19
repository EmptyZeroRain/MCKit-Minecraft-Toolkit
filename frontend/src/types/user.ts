// ===== 普通用户账号与个人数据（需求 §12）=====
// 对齐后端：/api/v1/user/*（见 backend/internal/http/handler/user.go）。

/** 可收藏 / 记录的对象类型 */
export type FavoriteKind =
  | 'tool'
  | 'item'
  | 'resource'
  | 'wiki'
  | 'potion'
  | 'villager'
  | 'mob'
  | 'redstone_design'
  | 'mod'
  | 'modpack'

/** 对象类型中文名（用于工作台分组展示） */
export const FAVORITE_KIND_LABELS: Record<FavoriteKind, string> = {
  tool: '工具',
  item: '物品',
  resource: '资源',
  wiki: '百科',
  potion: '药水',
  villager: '村民职业',
  mob: '生物',
  redstone_design: '机器设计',
  mod: 'Mod',
  modpack: '整合包',
}

/** 当前登录用户（后端不返回密码等敏感字段） */
export interface UserAccount {
  id: string
  username: string
  /** 写操作 CSRF token（登录 / me 时下发） */
  csrf_token?: string
  expires_at?: string
}

/** 收藏项 */
export interface UserFavoriteItem {
  id: string
  kind: FavoriteKind
  object_id: string
  label: string
  url: string
  created_at: string
}

/** 最近使用项 */
export interface UserRecentItem {
  id: string
  kind: FavoriteKind
  object_id: string
  label: string
  url: string
  visited_at: string
}

/** 常用种子 / 查询记录 */
export interface UserSeed {
  id: string
  name: string
  seed: string
  note: string
  created_at: string
  updated_at: string
}

export interface RegisterPayload {
  username: string
  password: string
}

export interface LoginPayload {
  username: string
  password: string
}

export interface ToggleFavoritePayload {
  kind: FavoriteKind
  object_id: string
  label?: string
  url?: string
}

export interface RecordRecentPayload {
  kind: FavoriteKind
  object_id: string
  label?: string
  url?: string
}

export interface SaveSeedPayload {
  /** 为空表示新建 */
  id?: string
  name: string
  seed: string
  note?: string
}

/** 用户名规则（与后端 usernameRe 保持一致：字母开头，3-40 位） */
export const USERNAME_RE = /^[A-Za-z][A-Za-z0-9_-]{2,39}$/
/** 用户密码最小长度（与后端 MinPasswordLength 一致） */
export const MIN_USER_PASSWORD_LENGTH = 8
