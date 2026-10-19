// ===== 我的世界开发栏目：开发文档知识库类型（TODO §21）=====
// 契约来源：backend/internal/domain/devdoc.go（枚举取值必须一致）。

/** 所属平台 / 加载器 */
export type DevLoader = 'neoforge' | 'forge' | 'fabric' | 'vanilla' | 'tooling' | 'multi'

/** 主题分类 */
export type DevCategory =
  | 'getting_started'
  | 'events'
  | 'registry'
  | 'networking'
  | 'datagen'
  | 'resources'
  | 'rendering'
  | 'serialization'
  | 'api_reference'
  | 'mapping'
  | 'toolchain'

/** 关键接口条目类型 */
export type DevApiKind = 'class' | 'method' | 'event' | 'field' | 'command' | 'file' | 'plugin'

/** 关键接口条目（类 / 方法 / 事件 / 字段 / 命令 / 文件） */
export interface DevApiEntry {
  name: string
  kind: DevApiKind
  /** 签名或用法片段，可为空 */
  signature: string
  /** 一句话说明 */
  note: string
}

/** 列表条目（不含正文，接口数量由后端计算） */
export interface DevDocSummary {
  slug: string
  title_zh: string
  title_en: string
  loader: DevLoader
  category: DevCategory
  summary: string
  /** 适用版本声明（「自某版本起」如 1.20.1+，或范围 / all） */
  mc_versions: string[]
  /** 兼容平台集合（multi 主题会列出多个） */
  loaders: DevLoader[]
  tags: string[]
  api_entry_count: number
  official_url: string
  sort_order: number
  last_verified_at: string | null
}

/** 文档详情 */
export interface DevDoc extends DevDocSummary {
  id: string
  body_md: string
  api_entries: DevApiEntry[]
  source_url: string
  status: 'draft' | 'published'
  data_version: string
  created_at: string
  updated_at: string
}

/** 详情响应：文档 + 同平台相关条目 */
export interface DevDocDetail {
  doc: DevDoc
  related: DevDocSummary[]
}

/** 查询参数 */
export interface DevDocQuery {
  q?: string
  loader?: DevLoader | ''
  category?: DevCategory | ''
  /** 目标游戏版本（如 1.21.1）；不传表示不限版本 */
  version?: string
  page?: number
  page_size?: number
}

/** 一个筛选维度的取值与计数 */
export interface DevFacet {
  value: string
  count: number
}

/** 筛选维度统计（由后端统计，前端不硬编码枚举） */
export interface DevDocFacets {
  loaders: DevFacet[]
  categories: DevFacet[]
  total: number
}

// ===== 展示用标签（与后端枚举一一对应）=====

export const DEV_LOADER_LABELS: Record<DevLoader, string> = {
  neoforge: 'NeoForge',
  forge: 'Forge',
  fabric: 'Fabric',
  vanilla: '原版数据',
  tooling: '工具链',
  multi: '跨平台',
}

export const DEV_LOADERS: DevLoader[] = [
  'neoforge',
  'forge',
  'fabric',
  'vanilla',
  'tooling',
  'multi',
]

export const DEV_CATEGORY_LABELS: Record<DevCategory, string> = {
  getting_started: '入门与结构',
  events: '事件与生命周期',
  registry: '注册表',
  networking: '网络',
  datagen: '数据生成',
  resources: '资源与数据文件',
  rendering: '渲染与界面',
  serialization: '序列化与存储',
  api_reference: 'API 速查',
  mapping: '映射与混淆',
  toolchain: '工具链',
}

export const DEV_CATEGORIES: DevCategory[] = [
  'getting_started',
  'events',
  'registry',
  'networking',
  'datagen',
  'resources',
  'rendering',
  'serialization',
  'api_reference',
  'mapping',
  'toolchain',
]

export const DEV_API_KIND_LABELS: Record<DevApiKind, string> = {
  class: '类',
  method: '方法',
  event: '事件',
  field: '字段',
  command: '命令',
  file: '文件',
  plugin: '插件',
}

/** 平台展示名（容错：未知取值回退原样） */
export function devLoaderLabel(loader: string): string {
  return DEV_LOADER_LABELS[loader as DevLoader] ?? loader
}

/** 主题展示名 */
export function devCategoryLabel(category: string): string {
  return DEV_CATEGORY_LABELS[category as DevCategory] ?? category
}

/** 接口类型展示名 */
export function devApiKindLabel(kind: string): string {
  return DEV_API_KIND_LABELS[kind as DevApiKind] ?? kind
}

/** 详情页可复制的接口速查文本（纯文本，便于粘进代码或笔记）。 */
export function apiEntryText(entry: DevApiEntry): string {
  const signature = entry.signature ? ` — ${entry.signature}` : ''
  const note = entry.note ? `（${entry.note}）` : ''
  return `${entry.name}${signature}${note}`
}
