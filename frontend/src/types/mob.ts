// ===== 生物资料类型（TODO §9C）=====
import type { ItemRef } from './item'

/** 生物类别（与后端 domain.MobCategories 契约一致） */
export type MobCategory = 'passive' | 'neutral' | 'hostile' | 'boss' | 'utility' | 'other'

/** 出现维度 */
export type MobDimension = 'overworld' | 'nether' | 'end'

/** 生物资料 */
export interface Mob {
  id: string
  slug: string
  name_zh: string
  name_en: string | null
  aliases: string[]
  category: MobCategory
  /** Java 版生命值（1 点 = 半颗心），未知为 null */
  health: number | null
  attack_note: string
  behavior_note: string
  spawn_note: string
  taming_note: string
  breeding_note: string
  dimensions: MobDimension[]
  biomes_note: string
  summary: string
  game_edition: 'java' | 'bedrock' | 'both'
  supported_versions: string[]
  source_url: string | null
  data_version: string | null
  last_verified_at: string | null
  sort_order: number
  created_at: string
  updated_at: string
}

/** 一条生物掉落 */
export interface MobDrop {
  id: string
  drop_key: string
  mob_slug: string
  item_id: string
  item_name_zh: string
  count_min: number
  count_max: number
  /** 掉落概率（0–1，1 表示必定掉落） */
  chance: number
  conditions: string
  note: string
  sort_order: number
  /** 该掉落物在物品库中的摘要；未收录时为 null，前端回退为纯文本 ID */
  item_ref: ItemRef | null
}

/** 生物详情：本体 + 掉落 + 关联物品 */
export interface MobDetail extends Mob {
  drops: MobDrop[]
  resolved_items: Record<string, ItemRef>
}

/** 生物列表查询参数 */
export interface MobQuery {
  q?: string
  category?: MobCategory | ''
  version?: string
  page?: number
  page_size?: number
}

/** 生物类别展示名与顺序（与后端 domain.MobCategories 一致）。 */
export const MOB_CATEGORY_LABELS: Record<MobCategory, string> = {
  passive: '被动型',
  neutral: '中立型',
  hostile: '敌对型',
  boss: 'Boss',
  utility: '功能型',
  other: '其他',
}

export const MOB_CATEGORIES: MobCategory[] = [
  'passive',
  'neutral',
  'hostile',
  'boss',
  'utility',
  'other',
]

/** 维度展示名。 */
export const MOB_DIMENSION_LABELS: Record<MobDimension, string> = {
  overworld: '主世界',
  nether: '下界',
  end: '末地',
}

/** 维度徽章配色。 */
export function dimensionClass(dim: MobDimension): string {
  switch (dim) {
    case 'nether':
      return 'border-red-500/40 bg-red-500/10 text-red-300'
    case 'end':
      return 'border-purple-500/40 bg-purple-500/10 text-purple-300'
    default:
      return 'border-grass-500/40 bg-grass-500/10 text-grass-300'
  }
}

/** 把 Java 版生命值换算为「心」的展示文本（1 点 = 半颗心）。 */
export function formatHealth(health: number | null): string {
  if (health === null || health === undefined) return '—'
  const hearts = health / 2
  return `${health}（${Number.isInteger(hearts) ? hearts : hearts.toFixed(1)} 颗心）`
}

/** 数量区间展示：固定数量只显示一个数字。 */
export function formatCount(min: number, max: number): string {
  return min === max ? String(min) : `${min}–${max}`
}

/** 概率展示：1 显示「必定」，其余显示百分比（非整数保留一位小数，避免稀有掉落失真）。 */
export function formatChance(chance: number): string {
  if (chance >= 1) return '必定'
  const pct = chance * 100
  return `${Number.isInteger(pct) ? pct : pct.toFixed(1)}%`
}
