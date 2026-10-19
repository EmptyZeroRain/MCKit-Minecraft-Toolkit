// ===== 物品 ID 对照表类型（移植自 CreeperCode）=====

/** 物品形式分类 */
export const BLOCK_ID_KINDS = [
  'same',
  'different',
  'variant',
  'overlay',
  'special',
  'none',
  'item',
  'other',
] as const
export type BlockIdKind = (typeof BLOCK_ID_KINDS)[number]

export const BLOCK_ID_KIND_LABELS: Record<BlockIdKind, string> = {
  same: 'ID 相同',
  different: 'ID 不同',
  variant: '变种',
  overlay: '叠加型',
  special: '特殊',
  none: '无物品形式',
  item: '纯物品',
  other: '其他',
}

/** 分类配色（对齐原 md 图例语义） */
export const BLOCK_ID_KIND_COLORS: Record<BlockIdKind, string> = {
  same: '#7cd15f',
  different: '#7dd3fc',
  variant: '#c084fc',
  overlay: '#f9a8d4',
  special: '#fbbf24',
  none: '#f87171',
  item: '#a3e635',
  other: '#9aa9c4',
}

/** 对照表条目 */
export interface BlockIdEntry {
  name: string
  id: string
  /** 物品形式描述（如「无物品形式」「与下方 ID 相同」等） */
  form: string
  kind: BlockIdKind
  /** 参考 ID（部分分类指向对应物品） */
  refId?: string
}

/** 对照表查询参数（全 POST + body） */
export interface BlockIdQuery {
  q?: string
  kind?: BlockIdKind | 'all'
}

/** 对照表响应 */
export interface BlockIdListResponse {
  total: number
  entries: BlockIdEntry[]
}