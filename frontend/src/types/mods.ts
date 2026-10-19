// ===== Mod 与整合包兼容性类型（TODO §16）=====
// 与后端 domain.Mod / domain.ModVersion / domain.ModDependency / domain.Modpack / domain.ModpackMod 契约一致。

/** Mod 加载器（与后端 domain.ModLoaders 一致；DB 不做 CHECK，由导入器校验） */
export type ModLoader = 'fabric' | 'forge' | 'neoforge' | 'quilt'

/** Mod 功能类别（与后端 domain.ModCategories 一致） */
export type ModCategory =
  | 'ui'
  | 'optimization'
  | 'technology'
  | 'magic'
  | 'world'
  | 'adventure'
  | 'library'
  | 'storage'
  | 'decoration'
  | 'other'

/** 发布版本类型 */
export type ModReleaseType = 'release' | 'beta' | 'alpha'

/** 依赖 / 冲突类型（本站只呈现**已记录**的关系） */
export type ModDependencyKind = 'required' | 'optional' | 'embedded' | 'incompatible'

/** 内容审核状态（仅 published 对公开接口可见） */
export type ModReviewStatus = 'pending' | 'published' | 'rejected'

/** 游戏版本归属 */
export type GameEdition = 'java' | 'bedrock' | 'both'

/** Mod 父条目 */
export interface Mod {
  id: string
  slug: string
  name_zh: string
  name_en: string | null
  aliases: string[]
  summary: string
  description: string
  categories: ModCategory[]
  loaders: ModLoader[]
  /** 适用游戏版本声明（单版本 + 范围，如 1.20-1.21 / 1.19+） */
  mc_versions: string[]
  homepage_url: string | null
  /** 官方仓库 / 主页，作为可核验锚点 */
  source_url: string | null
  license: string | null
  license_note: string
  game_edition: GameEdition
  data_version: string | null
  last_verified_at: string | null
  review_status: ModReviewStatus
  reviewer: string | null
  reviewed_at: string | null
  review_note: string
  sort_order: number
  created_at: string
  updated_at: string
}

/** 同一 Mod 的一个发布版本 */
export interface ModVersion {
  id: string
  mod_id: string
  version: string
  release_type: ModReleaseType
  mc_versions: string[]
  loaders: ModLoader[]
  /** 仅官方 / 作者授权分发入口；空串表示不提供链接 */
  download_url: string | null
  download_note: string
  file_name: string
  published_at: string | null
  source_url: string | null
  /** 验证状态：是否已核验该版本的来源与下载入口 */
  verified: boolean
  verify_note: string
  sort_order: number
  created_at: string
  updated_at: string
}

/** 关联展示用的 Mod 摘要（已收录才存在） */
export interface ModRef {
  slug: string
  name_zh: string
  name_en: string | null
  loaders: ModLoader[]
  url: string
}

/** 一条已记录的依赖 / 冲突 */
export interface ModDependency {
  id: string
  mod_id: string
  dep_kind: ModDependencyKind
  target_mod_id: string | null
  target_mod_name: string
  version_note: string
  note: string
  source_url: string | null
  verified: boolean
  created_at: string
  updated_at: string
  /** 已收录对方 Mod 的摘要；未收录为 null，前端回退纯文本名 */
  target_mod: ModRef | null
}

/** 反向依赖：谁依赖了当前 Mod */
export interface ModDependent {
  dep_kind: ModDependencyKind
  mod: ModRef
  version_note: string
  note: string
  source_url: string | null
  verified: boolean
}

/** Mod 详情：本体 + 发布版本 + 依赖与冲突 + 反向依赖 */
export interface ModDetail extends Mod {
  versions: ModVersion[]
  dependencies: ModDependency[]
  dependents: ModDependent[]
}

/** 整合包父条目 */
export interface Modpack {
  id: string
  slug: string
  name_zh: string
  name_en: string | null
  aliases: string[]
  summary: string
  description: string
  mc_version: string
  loader: ModLoader | ''
  author: string
  homepage_url: string | null
  source_url: string | null
  license: string | null
  license_note: string
  data_version: string | null
  last_verified_at: string | null
  review_status: ModReviewStatus
  reviewer: string | null
  reviewed_at: string | null
  review_note: string
  sort_order: number
  created_at: string
  updated_at: string
}

/** 整合包内一条 Mod 清单项 */
export interface ModpackMod {
  id: string
  modpack_id: string
  mod_id: string | null
  mod_name: string
  version: string
  note: string
  sort_order: number
  created_at: string
  /** 已收录 Mod 的摘要；未收录为 null，前端回退纯文本名 */
  mod_ref: ModRef | null
}

/** 整合包详情：本体 + 包内 Mod 清单 */
export interface ModpackDetail extends Modpack {
  mods: ModpackMod[]
}

/** Mod 列表查询参数 */
export interface ModQuery {
  q?: string
  loader?: ModLoader | ''
  category?: ModCategory | ''
  version?: string
  page?: number
  page_size?: number
}

/** 整合包列表查询参数 */
export interface ModpackQuery {
  q?: string
  loader?: ModLoader | ''
  version?: string
  page?: number
  page_size?: number
}

/** 加载器展示名（顺序即后端 domain.ModLoaders） */
export const MOD_LOADER_LABELS: Record<ModLoader, string> = {
  fabric: 'Fabric',
  forge: 'Forge',
  neoforge: 'NeoForge',
  quilt: 'Quilt',
}

export const MOD_LOADERS: ModLoader[] = ['fabric', 'forge', 'neoforge', 'quilt']

/** 类别展示名（顺序即后端 domain.ModCategories） */
export const MOD_CATEGORY_LABELS: Record<ModCategory, string> = {
  ui: '界面',
  optimization: '优化',
  technology: '科技',
  magic: '魔法',
  world: '世界',
  adventure: '冒险',
  library: '前置库',
  storage: '存储',
  decoration: '装饰',
  other: '其他',
}

export const MOD_CATEGORIES: ModCategory[] = [
  'ui',
  'optimization',
  'technology',
  'magic',
  'world',
  'adventure',
  'library',
  'storage',
  'decoration',
  'other',
]

/** 发布版本类型展示名 */
export const MOD_RELEASE_TYPE_LABELS: Record<ModReleaseType, string> = {
  release: '正式版',
  beta: '测试版',
  alpha: '内测版',
}

/** 依赖类型展示名（顺序即后端 domain.ModDependencyKinds） */
export const MOD_DEPENDENCY_KIND_LABELS: Record<ModDependencyKind, string> = {
  required: '必需前置',
  optional: '可选前置',
  embedded: '内嵌 / 捆绑',
  incompatible: '不兼容',
}

export const MOD_DEPENDENCY_KINDS: ModDependencyKind[] = [
  'required',
  'optional',
  'embedded',
  'incompatible',
]

/** 审核状态展示名 */
export const MOD_REVIEW_STATUS_LABELS: Record<ModReviewStatus, string> = {
  pending: '待审核',
  published: '已发布',
  rejected: '已驳回',
}

/** 游戏版本归属展示名 */
export const GAME_EDITION_LABELS: Record<GameEdition, string> = {
  java: 'Java 版',
  bedrock: '基岩版',
  both: 'Java 版 / 基岩版',
}

/** 加载器徽章配色。 */
export function loaderClass(loader: ModLoader): string {
  switch (loader) {
    case 'fabric':
      return 'border-amber-500/40 bg-amber-500/10 text-amber-300'
    case 'forge':
      return 'border-sky-500/40 bg-sky-500/10 text-sky-300'
    case 'neoforge':
      return 'border-orange-500/40 bg-orange-500/10 text-orange-300'
    case 'quilt':
      return 'border-purple-500/40 bg-purple-500/10 text-purple-300'
    default:
      return 'border-border-light bg-ink-700 text-text-muted'
  }
}

/** 类别徽章配色。 */
export function categoryClass(category: ModCategory): string {
  switch (category) {
    case 'optimization':
      return 'border-grass-500/40 bg-grass-500/10 text-grass-300'
    case 'technology':
      return 'border-sky-500/40 bg-sky-500/10 text-sky-300'
    case 'library':
      return 'border-purple-500/40 bg-purple-500/10 text-purple-300'
    case 'ui':
      return 'border-amber-500/40 bg-amber-500/10 text-amber-300'
    case 'storage':
      return 'border-orange-500/40 bg-orange-500/10 text-orange-300'
    case 'world':
      return 'border-emerald-500/40 bg-emerald-500/10 text-emerald-300'
    default:
      return 'border-border-light bg-ink-700 text-text-muted'
  }
}

/** 发布版本类型徽章配色。 */
export function releaseTypeClass(releaseType: ModReleaseType): string {
  switch (releaseType) {
    case 'release':
      return 'border-grass-500/40 bg-grass-500/10 text-grass-300'
    case 'beta':
      return 'border-amber-500/40 bg-amber-500/10 text-amber-300'
    default:
      return 'border-red-500/40 bg-red-500/10 text-red-300'
  }
}

/** 依赖类型徽章配色（不兼容用红色强调）。 */
export function dependencyKindClass(kind: ModDependencyKind): string {
  switch (kind) {
    case 'required':
      return 'border-sky-500/40 bg-sky-500/10 text-sky-300'
    case 'optional':
      return 'border-border-light bg-ink-700 text-text-muted'
    case 'embedded':
      return 'border-purple-500/40 bg-purple-500/10 text-purple-300'
    default:
      return 'border-red-500/40 bg-red-500/10 text-red-300'
  }
}

/** 是否为可安全外链的 http(s) 地址（避免生成无效链接）。 */
export function isExternalUrl(url: string | null | undefined): boolean {
  return !!url && /^https?:\/\//i.test(url)
}

/** 验证状态展示名。 */
export function verifiedLabel(verified: boolean): string {
  return verified ? '已验证' : '未验证'
}

/** 验证状态徽章配色。 */
export function verifiedClass(verified: boolean): string {
  return verified
    ? 'border-grass-500/40 bg-grass-500/10 text-grass-300'
    : 'border-border-light bg-ink-700 text-text-muted'
}

/**
 * 版本的可下载链接：**仅在同时满足「官方授权入口」与「已说明授权」时返回 URL**。
 * 对应 §16 版权红线：只链接官方 / 作者授权的分发入口，不未经许可镜像分发文件。
 * 无授权说明时返回 null，前端回退为 download_note 文字说明。
 */
export function authorizedDownloadUrl(
  version: Pick<ModVersion, 'download_url'>,
  mod: Pick<Mod, 'license' | 'license_note'>,
): string | null {
  if (!isExternalUrl(version.download_url)) return null
  const licensed =
    !!(mod.license && mod.license.trim()) || !!(mod.license_note && mod.license_note.trim())
  return licensed ? (version.download_url as string) : null
}

/** 整合包清单项展示名：已收录优先中文名，未收录回退纯文本名。 */
export function modpackModLabel(m: Pick<ModpackMod, 'mod_ref' | 'mod_name'>): string {
  return m.mod_ref?.name_zh || m.mod_name || '未命名'
}

/** 整合包清单项链接：已收录才给链接，未收录返回 null（不生成死链）。 */
export function modpackModUrl(m: Pick<ModpackMod, 'mod_ref'>): string | null {
  return m.mod_ref?.url ?? null
}

/** 依赖目标展示名：已收录优先中文名，未收录回退纯文本名。 */
export function dependencyTargetLabel(d: Pick<ModDependency, 'target_mod' | 'target_mod_name'>): string {
  return d.target_mod?.name_zh || d.target_mod_name || '未知'
}
