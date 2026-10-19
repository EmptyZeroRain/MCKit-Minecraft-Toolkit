// ===== MC 版本信息类型（移植自 CreeperCode）=====

/** 版本类型 */
export const VERSION_TYPES = ['release', 'snapshot', 'old_beta', 'old_alpha'] as const
export type VersionType = (typeof VERSION_TYPES)[number]

/**
 * 「所有版本」通配声明（TODO §6-C）。
 *
 * 与后端 `versioning.AllKeyword` 对齐：资料的适用版本声明为 `all` 时
 * 表示适用于全部 MC 版本；该声明必须独占出现。
 */
export const ALL_VERSIONS = 'all'
export const ALL_VERSIONS_LABEL = '全部 MC 版本'

export const VERSION_TYPE_LABELS: Record<VersionType, string> = {
  release: '正式版',
  snapshot: '快照',
  old_beta: '旧测试版',
  old_alpha: '旧内测版',
}

/** 版本清单条目 */
export interface VersionEntry {
  id: string
  type: VersionType
  url: string
  time: string
  releaseTime: string
  sha1?: string
}

/** 最新版本指针 */
export interface LatestVersions {
  release: string
  snapshot: string
}

/** 版本列表响应 */
export interface VersionListResponse {
  latest: LatestVersions
  versions: VersionEntry[]
}

/** 下载项 */
export interface DownloadItem {
  name: string
  url: string
  size: number
  sha1: string
}

/** Java 版本要求 */
export interface JavaRequirement {
  component: string
  majorVersion: number
}

/** 库的平台原生变体 */
export interface NativeVariant {
  classifier: string
  url: string
  size: number
  sha1: string
}

/** 运行库 */
export interface LibraryInfo {
  name: string
  url: string
  size: number
  sha1: string
  /** 平台限制说明，空字符串表示全平台 */
  platforms?: string
  natives?: NativeVariant[]
}

/** 启动参数（含平台标注） */
export interface LaunchArg {
  value: string
  platforms?: string
}

/** 客户端启动命令模板 */
export interface ClientLaunch {
  mainClass: string
  command: string
  gameArgs: LaunchArg[]
  jvmArgs: LaunchArg[]
  defaultUserJVM?: LaunchArg[]
}

/** 版本详情 */
export interface VersionDetail {
  id: string
  type: VersionType
  releaseTime: string
  mainClass: string
  java: JavaRequirement
  downloads: DownloadItem[]
  libraries: LibraryInfo[]
  startupCommand: string
  startupNotes: string[]
  clientLaunch?: ClientLaunch
}

/** 版本列表查询参数（全 POST + body） */
export interface VersionListQuery {
  type?: VersionType | 'all'
  q?: string
}

/** 版本详情查询参数 */
export interface VersionDetailQuery {
  id: string
}