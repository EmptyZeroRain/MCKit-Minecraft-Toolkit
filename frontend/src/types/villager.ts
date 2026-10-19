// ===== 村民交易资料类型（TODO §9B）=====
import type { ItemRef } from './item'

/** 村民职业类别（与后端 domain.VillagerKinds 契约一致） */
export type VillagerKind = 'normal' | 'wandering' | 'non_trading'

/** 交易分组：随等级解锁 / 流浪商人收购 / 流浪商人特殊 / 流浪商人普通 */
export type VillagerTradeGroupKind = 'level' | 'purchase' | 'special' | 'ordinary'

/** 交易中「村民想要」的单条物品（数量为区间，固定数量时 min === max） */
export interface VillagerCostItem {
  item_id: string
  name_zh: string
  count_min: number
  count_max: number
}

/** 村民职业资料 */
export interface VillagerProfession {
  id: string
  slug: string
  name_zh: string
  name_en: string | null
  aliases: string[]
  kind: VillagerKind
  workstation_item_id: string | null
  workstation_name_zh: string
  summary: string
  description: string
  trade_note: string
  game_edition: 'java' | 'bedrock' | 'both'
  supported_versions: string[]
  source_url: string | null
  data_version: string | null
  last_verified_at: string | null
  sort_order: number
  created_at: string
  updated_at: string
}

/** 职业等级定义（通用规则，存于数据库） */
export interface VillagerTradeLevel {
  id: string
  level: number
  slug: string
  name_zh: string
  name_en: string | null
  required_experience: number
  badge_note: string
}

/** 一条村民交易 */
export interface VillagerTrade {
  id: string
  trade_key: string
  profession_slug: string
  /** 0 表示不分等级（流浪商人） */
  level: number
  trade_group: VillagerTradeGroupKind
  cost_items: VillagerCostItem[]
  result_item_id: string
  result_item_name_zh: string
  result_count: number
  max_uses: number
  villager_experience: number
  price_multiplier: number
  variant_note: string
  note: string
  game_edition: string
  supported_versions: string[]
  source_url: string | null
  sort_order: number
  /** 本条交易成本物品中确实已收录的物品摘要（键为 item_id） */
  cost_item_refs?: Record<string, ItemRef>
}

/** 按（等级 / 分组）聚合的交易集合 */
export interface VillagerTradeGroup {
  level: number
  level_slug: string
  level_name_zh: string
  required_experience: number
  trade_group: VillagerTradeGroupKind
  trades: VillagerTrade[]
}

/** 村民职业详情（本体 + 等级规则 + 分组交易 + 关联物品） */
export interface VillagerProfessionDetail extends VillagerProfession {
  levels: VillagerTradeLevel[]
  groups: VillagerTradeGroup[]
  trades: VillagerTrade[]
  workstation_item: ItemRef | null
  /** 全部交易引用到的、确实已收录的物品摘要（键为 item_id） */
  resolved_items?: Record<string, ItemRef>
}

/** 村民职业查询参数 */
export interface VillagerQuery {
  q?: string
  kind?: VillagerKind | ''
  /** 目标游戏版本（如 1.20）；不传表示不限版本 */
  version?: string
  page?: number
  page_size?: number
}

// ===== 展示用标签与格式化（与后端契约一一对应）=====

/** 职业类别展示名 */
export const VILLAGER_KIND_LABELS: Record<VillagerKind, string> = {
  normal: '可交易职业',
  wandering: '流浪商人',
  non_trading: '不参与交易',
}

/** 职业类别展示顺序（与后端排序一致） */
export const VILLAGER_KINDS: VillagerKind[] = ['normal', 'wandering', 'non_trading']

/** 交易分组展示名 */
export const VILLAGER_TRADE_GROUP_LABELS: Record<VillagerTradeGroupKind, string> = {
  level: '职业等级交易',
  purchase: '收购（换绿宝石）',
  special: '特殊交易',
  ordinary: '普通售卖',
}

/** 数量区间展示：固定数量显示单个数字，区间显示 19–33。 */
export function formatCostCount(item: VillagerCostItem): string {
  if (item.count_max > item.count_min) return `${item.count_min}–${item.count_max}`
  return `${item.count_min}`
}
