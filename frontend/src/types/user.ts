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
  /** 对外随机用户 ID（如 U7K3M2QX9F），用于展示与社区作者署名 */
  user_no?: string
  /** 账号角色：user=普通用户；admin/editor=后台账号（由 /user/me 返回） */
  role?: string
  username: string
  /** 展示昵称（注册必填；空串时前端回退到 username） */
  nickname?: string
  /** 头像图片（图片 data URI，仅上传图片；空串表示未设置） */
  avatar_url?: string
  /** 注册邮箱（作为可选登录标识） */
  email?: string
  /** 写操作 CSRF token（登录 / me 时下发） */
  csrf_token?: string
  expires_at?: string
}

/** 更新当前用户展示资料（昵称 / 头像） */
export interface UpdateProfilePayload {
  nickname: string
  /** 头像图片 data URI（空串表示清除）；仅接受图片，由前端压缩生成 */
  avatar_url: string
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
  /** 展示昵称（1-60 个字符，必填） */
  nickname: string
  /** 注册邮箱（全局唯一，可作为登录标识） */
  email: string
  password: string
  /** 图形验证码一次性 ID 与用户填写答案 */
  captcha_id: string
  captcha_answer: string
}

export interface LoginPayload {
  /** 登录标识：用户名或邮箱 */
  identifier: string
  password: string
}

/** 图形验证码挑战（服务端签发） */
export interface CaptchaChallenge {
  captcha_id: string
  /** PNG 图片的 data URI，可直接用于 <img :src> */
  image: string
  /** 有效期（秒） */
  expires_in: number
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
/** 邮箱规则（与后端 emailRe 保持一致：本地部分 + 域名 + 顶级域） */
export const EMAIL_RE = /^[A-Za-z0-9._%+-]+@([A-Za-z0-9-]+\.)+[A-Za-z]{2,}$/
/** 昵称最大长度（与后端 MaxNicknameLength、accounts.nickname 列宽一致） */
export const MAX_NICKNAME_LENGTH = 60
/** 用户密码最小长度（与后端 MinPasswordLength 一致） */
export const MIN_USER_PASSWORD_LENGTH = 8
