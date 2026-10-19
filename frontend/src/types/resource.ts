// ===== 资源实体类型 =====
import type {
  LinkStatus,
  LoaderType,
  ResourceStatus,
  ResourceType,
} from './common'

/** 依赖关系类型（TODO §6）：依赖 / 前置 Mod / 已知冲突 */
export const DEPENDENCY_TYPES = ['prerequisite', 'dependency', 'conflict'] as const
export type ResourceDependencyType = (typeof DEPENDENCY_TYPES)[number]

/** 依赖描述 */
export interface ResourceDependency {
  name: string
  /** 缺省视为 dependency（兼容历史数据） */
  type?: ResourceDependencyType
  resource_url?: string
  required?: boolean
  note?: string
}

/** 发布版本：同一资源的不同发布版本（TODO §6） */
export interface ResourceRelease {
  id: string
  resource_id: string
  version_label: string
  game_versions: string[]
  loader_types: LoaderType[]
  source_url: string
  /** 发布时间；为空表示未知，不做猜测 */
  released_at: string | null
  notes: string
  is_latest: boolean
  created_at: string
  updated_at: string
}

/**
 * 兼容性结论（TODO §6）。
 * `verified=false` 表示缺少可靠数据依据，界面必须明确展示「未验证」，
 * 不得把缺失数据当作兼容。
 */
export interface ResourceCompatibility {
  verified: boolean
  game_versions: string[]
  loader_types: LoaderType[]
  /** 结论依据来源 */
  sources: Array<'resource' | 'releases'>
  /** 是否声明了「所有版本」通配（game_versions 含 all） */
  all_versions?: boolean
  /** 人工核验备注 */
  note: string
  /** 未验证时的说明文案 */
  unverified_hint?: string
}

export interface Resource {
  id: string
  slug: string
  name: string
  summary: string
  description: string
  resource_type: ResourceType
  cover_image_url: string | null
  author_name: string | null
  source_platform: string | null
  source_url: string
  license: string | null
  game_edition: 'java' | 'bedrock'
  supported_versions: string[]
  loader_types: LoaderType[]
  dependencies: ResourceDependency[]
  install_guide: string
  tags: string[]
  /** 人工核验备注（可选） */
  compatibility_note: string
  status: ResourceStatus
  link_status: LinkStatus
  last_checked_at: string | null
  published_at: string | null
  created_by: string | null
  updated_by: string | null
  created_at: string
  updated_at: string
}

/** 资源详情：资源本体 + 发布版本 + 兼容性结论（公开 / 管理端共用） */
export interface ResourceDetail extends Resource {
  releases: ResourceRelease[]
  compatibility: ResourceCompatibility
}

/** 发布版本写入载荷（管理端整体替换） */
export type ResourceReleasePayload = Omit<ResourceRelease, 'id' | 'resource_id' | 'created_at' | 'updated_at' | 'released_at'> & {
  released_at?: string | null
}

/** 资源列表查询参数（公开 + 管理端共用） */
export interface ResourceListQuery {
  q?: string
  type?: ResourceType | ''
  game_version?: string
  loader?: LoaderType | ''
  source_platform?: string
  sort?: 'updated_desc' | 'name_asc'
  status?: ResourceStatus | ''
  page?: number
  page_size?: number
}

/** 创建/更新资源时使用 */
export type ResourcePayload = Omit<
  Resource,
  | 'id'
  | 'link_status'
  | 'last_checked_at'
  | 'published_at'
  | 'created_by'
  | 'updated_by'
  | 'created_at'
  | 'updated_at'
> & {
  status?: ResourceStatus
}

/** 链接检查结果 */
export interface LinkCheckResult {
  url: string
  status: LinkStatus
  http_status?: number
  final_url?: string
  checked_at: string
  message?: string
}