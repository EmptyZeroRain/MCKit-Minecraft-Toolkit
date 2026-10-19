// ===== 药水与酿造资料类型（TODO §9A）=====
import type { ItemRef } from './item'

/** 药水类别（与后端 domain.PotionCategories 契约一致） */
export type PotionCategory = 'water' | 'base' | 'positive' | 'negative' | 'mixed' | 'other'

/** 效果变体：普通 / 红石延长 / 萤石增强 */
export type PotionVariant = 'normal' | 'extended' | 'enhanced'

/** 酿造步骤类型：基底 / 效果 / 腐化 / 通用修饰 / 形态转换 */
export type BrewStepKind = 'base' | 'effect' | 'corrupt' | 'modifier' | 'form'

/** 药水单条效果（同一药水的普通/延长/增强变体各一条） */
export interface PotionEffect {
  variant: PotionVariant
  effect_id: string
  name_zh: string
  name_en: string
  /** 等级：1 表示 I 级 */
  amplifier: number
  /** 持续时间（秒）；0 表示瞬间效果 */
  duration_seconds: number
  description: string
}

/** 药水资料 */
export interface Potion {
  id: string
  slug: string
  name_zh: string
  name_en: string | null
  aliases: string[]
  category: PotionCategory
  summary: string
  effect_note: string
  is_brewable: boolean
  is_extendable: boolean
  is_enhanceable: boolean
  effects: PotionEffect[]
  game_edition: 'java' | 'bedrock' | 'both'
  /** 适用版本声明（「自某版本起」如 1.8+，或范围 / all） */
  supported_versions: string[]
  source_url: string | null
  data_version: string | null
  last_verified_at: string | null
  created_at: string
  updated_at: string
}

/** 关联展示用的药水摘要 */
export interface PotionRef {
  slug: string
  name_zh: string
  category: PotionCategory
  url: string
}

/** 酿造步骤 */
export interface BrewingStep {
  id: string
  recipe_key: string
  step_kind: BrewStepKind
  input_potion_slug: string | null
  ingredient_item_id: string
  ingredient_name_zh: string
  result_potion_slug: string | null
  note: string
  game_edition: string
  supported_versions: string[]
  source_url: string | null
  sort_order: number
  input_potion: PotionRef | null
  result_potion: PotionRef | null
}

/** 药水详情（本体 + 酿造步骤 + 通用规则 + 关联物品） */
export interface PotionDetail extends Potion {
  produced_by: BrewingStep[]
  follow_ups: BrewingStep[]
  rules: BrewingStep[]
  /** 步骤原料中确实已收录的物品摘要（键为 item_id）；未收录者需回退为纯文本 ID */
  ingredient_items?: Record<string, ItemRef>
}

/** 通用酿造规则（红石延长 / 萤石增强 / 火药喷溅 / 龙息滞留） */
export interface BrewingRules {
  rules: BrewingStep[]
  ingredient_items?: Record<string, ItemRef>
}

/** 药水查询参数 */
export interface PotionQuery {
  q?: string
  category?: PotionCategory | ''
  /** 目标游戏版本（如 1.20）；不传表示不限版本 */
  version?: string
  page?: number
  page_size?: number
}

// ===== 展示用标签与格式化（与后端契约一一对应）=====

/** 类别展示名 */
export const POTION_CATEGORY_LABELS: Record<PotionCategory, string> = {
  water: '水瓶',
  base: '基础药水',
  positive: '增益药水',
  negative: '负面药水',
  mixed: '混合药水',
  other: '其他',
}

/** 类别展示顺序（与后端排序一致） */
export const POTION_CATEGORIES: PotionCategory[] = [
  'water',
  'base',
  'positive',
  'negative',
  'mixed',
  'other',
]

/** 变体展示名 */
export const POTION_VARIANT_LABELS: Record<PotionVariant, string> = {
  normal: '普通',
  extended: '延长（红石）',
  enhanced: '增强（荧石）',
}

/** 酿造步骤类型展示名 */
export const BREW_STEP_LABELS: Record<BrewStepKind, string> = {
  base: '基底酿造',
  effect: '效果酿造',
  corrupt: '腐化',
  modifier: '通用修饰',
  form: '形态转换',
}

/** 版本归属展示名 */
export const POTION_EDITION_LABELS: Record<Potion['game_edition'], string> = {
  java: '仅 Java 版',
  bedrock: '仅基岩版',
  both: 'Java 版与基岩版',
}

/** 罗马数字（等级展示）；超出常规范围时回退为「N 级」。 */
export function romanNumeral(value: number): string {
  const table: Array<[number, string]> = [
    [10, 'X'],
    [9, 'IX'],
    [5, 'V'],
    [4, 'IV'],
    [1, 'I'],
  ]
  if (value < 1 || value > 10) return `${value} 级`
  let rest = value
  let out = ''
  for (const [num, sym] of table) {
    while (rest >= num) {
      out += sym
      rest -= num
    }
  }
  return out
}

/** 持续时间展示：0 秒为「瞬间」，其余为 m:ss。 */
export function formatDuration(seconds: number): string {
  if (seconds <= 0) return '瞬间'
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return `${m}:${String(s).padStart(2, '0')}`
}

/** 单个效果的简述（如「迅捷 II · 1:30」）。 */
export function effectSummary(effect: PotionEffect): string {
  const level = romanNumeral(effect.amplifier)
  const time = effect.duration_seconds > 0 ? ` · ${formatDuration(effect.duration_seconds)}` : ''
  return `${effect.name_zh} ${level}${time}`
}

/** 列表用的效果摘要：优先展示普通变体，无普通变体时回退首条。 */
export function potionEffectSummary(potion: Potion): string {
  if (!potion.effects.length) return '无效果'
  const normal = potion.effects.filter((e) => e.variant === 'normal')
  const chosen = normal.length ? normal : potion.effects
  return chosen.map(effectSummary).join('，')
}
