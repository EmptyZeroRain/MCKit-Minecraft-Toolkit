// ===== 版本映射查询类型（TODO §21）=====
// 契约来源：backend/internal/domain/devdoc.go + internal/service/catalog/mappings.go。

/** 映射类型（数据集来源格式） */
export type MappingType = 'mojang' | 'srg' | 'tsrg' | 'yarn' | 'parchment' | 'mixed'

/** 成员类型 */
export type MappingMemberKind = 'method' | 'field' | 'param'

/** 查询范围 */
export type MappingSearchKind = 'all' | 'class' | 'method' | 'field'

/** 一套映射数据集 */
export interface MappingSet {
  id: string
  set_key: string
  mc_version: string
  mapping_type: MappingType
  channel: string
  build_label: string
  class_count: number
  member_count: number
  source_url: string
  license: string
  note: string
  imported_at: string | null
  created_at: string
  updated_at: string
}

/** 成员视图（不含仅用于去重的 desc_key） */
export interface MappingMember {
  kind: MappingMemberKind
  obf_name: string
  mid_name: string
  named_name: string
  obf_desc: string
  named_desc: string
  side: string
}

/** 类命中结果 */
export interface MappingClassHit {
  id: string
  set_id: string
  set_key: string
  mc_version: string
  mapping_type: MappingType
  obf_name: string
  mid_name: string
  named_name: string
  package: string
  member_count: number
  /** 命中的成员预览（最多 8 条） */
  matched_members: MappingMember[]
  /** 命中的成员总数（可能大于预览条数） */
  matched_total: number
}

/** 类详情：类映射 + 成员列表 */
export interface MappingClassDetail {
  class: MappingClassHit
  members: MappingMember[]
  member_total: number
  set: MappingSet | null
}

/** 数据集查询参数 */
export interface MappingSetQuery {
  q?: string
  mc_version?: string
  mapping_type?: MappingType | ''
}

/** 映射查询参数 */
export interface MappingSearchQuery {
  q?: string
  set_id?: string
  mc_version?: string
  mapping_type?: MappingType | ''
  kind?: MappingSearchKind
  page?: number
  page_size?: number
}

/**
 * 映射查询响应。
 * 说明：该接口未使用通用分页响应，而是把「命中集是否被扫描上限截断」
 * 与分页字段一并放在 data 中（业务语义，非纯分页元信息）。
 */
export interface MappingSearchResponse {
  hits: MappingClassHit[]
  total: number
  truncated: boolean
  page: number
  page_size: number
  total_pages: number
}

// ===== 展示用标签 =====

export const MAPPING_TYPE_LABELS: Record<MappingType, string> = {
  mojang: 'Mojang 官方映射',
  srg: 'SRG',
  tsrg: 'TSRG / TSRG2',
  yarn: 'Fabric Yarn',
  parchment: 'Parchment',
  mixed: '其它来源',
}

export const MAPPING_TYPES: MappingType[] = [
  'mojang',
  'srg',
  'tsrg',
  'yarn',
  'parchment',
  'mixed',
]

export const MAPPING_MEMBER_KIND_LABELS: Record<MappingMemberKind, string> = {
  method: '方法',
  field: '字段',
  param: '参数',
}

/** 查询范围选项（含「全部」） */
export const MAPPING_SEARCH_KIND_OPTIONS: Array<{ value: MappingSearchKind; label: string }> = [
  { value: 'all', label: '全部' },
  { value: 'class', label: '仅类名' },
  { value: 'method', label: '仅方法' },
  { value: 'field', label: '仅字段' },
]

/** 映射类型展示名 */
export function mappingTypeLabel(type: string): string {
  return MAPPING_TYPE_LABELS[type as MappingType] ?? type
}

/** 成员类型展示名 */
export function mappingMemberKindLabel(kind: string): string {
  return MAPPING_MEMBER_KIND_LABELS[kind as MappingMemberKind] ?? kind
}

/** 取类 / 成员最可读的名字：named > mid > obf（三者都可能为空）。 */
export function displayName(row: {
  named_name?: string
  mid_name?: string
  obf_name?: string
}): string {
  return row.named_name || row.mid_name || row.obf_name || '—'
}

/** 类的短名（去掉包路径）。 */
export function shortClassName(name: string): string {
  if (!name) return '—'
  const parts = name.replace(/\./g, '/').split('/')
  return parts[parts.length - 1] || name
}
