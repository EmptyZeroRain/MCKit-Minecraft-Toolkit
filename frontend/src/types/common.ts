// ===== 全站共享枚举与常量 =====

/** 游戏版本：首期 Java 版；保留字段便于扩展基岩版 */
export type GameEdition = 'java' | 'bedrock'

/** 工具分类 */
export const TOOL_CATEGORIES = [
  '查询类',
  '计算类',
  '生成类',
  '编辑类',
  '辅助类',
] as const
export type ToolCategory = (typeof TOOL_CATEGORIES)[number]

/** 资源类型 */
export const RESOURCE_TYPES = [
  'mod',
  'modpack',
  'resource_pack',
  'shader',
  'world',
  'datapack',
  'plugin',
] as const
export type ResourceType = (typeof RESOURCE_TYPES)[number]

export const RESOURCE_TYPE_LABELS: Record<ResourceType, string> = {
  mod: 'Mod',
  modpack: '整合包',
  resource_pack: '资源包',
  shader: '光影',
  world: '存档',
  datapack: '数据包',
  plugin: '插件',
}

/** 加载器 */
export const LOADER_TYPES = [
  'forge',
  'neoforge',
  'fabric',
  'quilt',
  'vanilla',
  'paper',
  'spigot',
  'purpur',
  'other',
] as const
export type LoaderType = (typeof LOADER_TYPES)[number]

export const LOADER_LABELS: Record<LoaderType, string> = {
  forge: 'Forge',
  neoforge: 'NeoForge',
  fabric: 'Fabric',
  quilt: 'Quilt',
  vanilla: '原版',
  paper: 'Paper',
  spigot: 'Spigot',
  purpur: 'Purpur',
  other: '其他',
}

/** 资源状态 */
export const RESOURCE_STATUSES = ['draft', 'published', 'hidden', 'removed'] as const
export type ResourceStatus = (typeof RESOURCE_STATUSES)[number]

/** 链接检查状态 */
export const LINK_STATUSES = [
  'unchecked',
  'ok',
  'redirected',
  'broken',
  'blocked',
  'error',
] as const
export type LinkStatus = (typeof LINK_STATUSES)[number]

export const LINK_STATUS_LABELS: Record<LinkStatus, string> = {
  unchecked: '未检查',
  ok: '正常',
  redirected: '已重定向',
  broken: '失效',
  blocked: '被拦截',
  error: '检查出错',
}

/** 百科内容类型 */
export type WikiContentType = 'guide' | 'concept' | 'tutorial' | 'reference' | 'other'

/** 反馈 / 投稿类型（与后端 domain.FeedbackTypes 契约一致） */
export const FEEDBACK_TYPES = [
  'bug',
  'resource_broken',
  'infringement',
  'data_correction',
  'resource_supplement',
  'other',
] as const
export type FeedbackType = (typeof FEEDBACK_TYPES)[number]

export const FEEDBACK_TYPE_LABELS: Record<FeedbackType, string> = {
  bug: '错误反馈',
  resource_broken: '资源失效',
  infringement: '侵权通知',
  data_correction: '资料纠错',
  resource_supplement: '资源补充信息',
  other: '其他',
}

/**
 * 需要接受授权条款的投稿类型（TODO §17：明确投稿内容的授权条款与版权处理流程）。
 * 资料纠错与资源补充属于「向本站贡献内容」，须先确认授权。
 */
export const FEEDBACK_LICENSE_TYPES: readonly FeedbackType[] = ['data_correction', 'resource_supplement']

/** 当前投稿授权条款版本（与后端 domain.FeedbackLicenseVersion 一致） */
export const FEEDBACK_LICENSE_VERSION = '2026-10-18'

/** 投稿关联的资料条目类型（与后端 domain.FeedbackTargetKinds 契约一致） */
export const FEEDBACK_TARGET_KINDS = [
  'resource',
  'item',
  'dev_doc',
  'mob',
  'villager',
  'potion',
  'tool',
  'other',
] as const
export type FeedbackTargetKind = (typeof FEEDBACK_TARGET_KINDS)[number]

export const FEEDBACK_TARGET_KIND_LABELS: Record<FeedbackTargetKind, string> = {
  resource: '资源',
  item: '物品',
  dev_doc: '开发文档',
  mob: '生物',
  villager: '村民职业',
  potion: '药水',
  tool: '工具',
  other: '其他 / 不指定',
}

/** 反馈状态 */
export const FEEDBACK_STATUSES = ['new', 'processing', 'resolved', 'ignored'] as const
export type FeedbackStatus = (typeof FEEDBACK_STATUSES)[number]

export const FEEDBACK_STATUS_LABELS: Record<FeedbackStatus, string> = {
  new: '新提交',
  processing: '处理中',
  resolved: '已解决',
  ignored: '已忽略',
}

/** 配方类型 */
export const RECIPE_TYPES = [
  'shaped',
  'shapeless',
  'smelting',
  'blasting',
  'smoking',
  'campfire_cooking',
  'stonecutting',
  'smithing',
  'other',
] as const
export type RecipeType = (typeof RECIPE_TYPES)[number]

export const RECIPE_TYPE_LABELS: Record<RecipeType, string> = {
  shaped: '有序合成',
  shapeless: '无序合成',
  smelting: '熔炼',
  blasting: '高炉烧炼',
  smoking: '烟熏',
  campfire_cooking: '营火烹饪',
  stonecutting: '切石',
  smithing: '锻造',
  other: '其他',
}

/** 通用状态：发布相关 */
export type PublishStatus = 'published' | 'draft'

/** 内置后台角色标识；自定义角色为任意标识，故 AdminRole 放宽为 string（真实取值以 roles 表为准） */
export const BUILTIN_ADMIN_ROLES = ['admin', 'editor'] as const
export type AdminRole = string