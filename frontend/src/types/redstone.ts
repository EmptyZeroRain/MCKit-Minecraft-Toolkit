// ===== 红石与机器设计资料库类型（TODO §15）=====
// 与后端 domain.RedstoneComponent / domain.RedstoneDesign 契约一致。
import type { ItemRef } from './item'

/** 红石元件类别（与后端 domain.RedstoneComponentCategories 一致） */
export type RedstoneComponentCategory =
  | 'source'
  | 'transmission'
  | 'logic'
  | 'actuator'
  | 'sensor'
  | 'other'

/** 机器设计用途（与后端 domain.RedstoneDesignPurposes 一致） */
export type RedstoneDesignPurpose =
  | 'farm'
  | 'sorter'
  | 'transport'
  | 'door'
  | 'logic'
  | 'utility'
  | 'other'

/** 机器设计难度 */
export type RedstoneDesignDifficulty = 'easy' | 'medium' | 'hard'

/** 内容审核状态（仅 published 对公开接口可见） */
export type RedstoneReviewStatus = 'pending' | 'published' | 'rejected'

/** 适用版本 */
export type GameEdition = 'java' | 'bedrock' | 'both'

/** 红石元件资料 */
export interface RedstoneComponent {
  id: string
  slug: string
  name_zh: string
  name_en: string | null
  aliases: string[]
  category: RedstoneComponentCategory
  /** 核心工作机制说明（信号强度 / 延迟等不确定数值以文字描述） */
  mechanism_note: string
  signal_note: string
  summary: string
  game_edition: GameEdition
  supported_versions: string[]
  source_url: string | null
  data_version: string | null
  last_verified_at: string | null
  sort_order: number
  created_at: string
  updated_at: string
}

/** 机器设计材料条目（数量区间；item_id 为空串表示未收录物品） */
export interface RedstoneMaterial {
  item_id: string
  name_zh: string
  count_min: number
  count_max: number
}

/** 机器设计资料（列表条目） */
export interface RedstoneDesign {
  id: string
  slug: string
  name_zh: string
  name_en: string | null
  aliases: string[]
  purpose: RedstoneDesignPurpose
  difficulty: RedstoneDesignDifficulty
  summary: string
  steps_md: string
  materials: RedstoneMaterial[]
  author: string
  author_url: string | null
  screenshot_url: string | null
  video_url: string | null
  /** 存档下载地址；非空时 license 必填（版权红线） */
  world_download_url: string | null
  world_download_note: string
  license: string | null
  license_note: string
  supported_versions: string[]
  game_edition: GameEdition
  source_url: string | null
  data_version: string | null
  last_verified_at: string | null
  review_status: RedstoneReviewStatus
  reviewer: string | null
  reviewed_at: string | null
  review_note: string
  sort_order: number
  created_at: string
  updated_at: string
}

/** 材料展示视图：附带已收录物品的摘要引用（未收录为 null） */
export interface RedstoneMaterialView extends RedstoneMaterial {
  item_ref: ItemRef | null
}

/** 机器设计详情：本体 + 材料 + 关联物品 */
export interface RedstoneDesignDetail extends RedstoneDesign {
  materials: RedstoneMaterialView[]
  resolved_items: Record<string, ItemRef>
}

/** 红石元件列表查询参数 */
export interface RedstoneComponentQuery {
  q?: string
  category?: RedstoneComponentCategory | ''
  version?: string
  page?: number
  page_size?: number
}

/** 机器设计列表查询参数 */
export interface RedstoneDesignQuery {
  q?: string
  purpose?: RedstoneDesignPurpose | ''
  difficulty?: RedstoneDesignDifficulty | ''
  version?: string
  page?: number
  page_size?: number
}

/** 元件类别展示名（顺序即后端 domain.RedstoneComponentCategories） */
export const REDSTONE_COMPONENT_CATEGORY_LABELS: Record<RedstoneComponentCategory, string> = {
  source: '信号源',
  transmission: '传输',
  logic: '逻辑',
  actuator: '执行',
  sensor: '传感',
  other: '其他',
}

export const REDSTONE_COMPONENT_CATEGORIES: RedstoneComponentCategory[] = [
  'source',
  'transmission',
  'logic',
  'actuator',
  'sensor',
  'other',
]

/** 设计用途展示名（顺序即后端 domain.RedstoneDesignPurposes） */
export const REDSTONE_DESIGN_PURPOSE_LABELS: Record<RedstoneDesignPurpose, string> = {
  farm: '农场',
  sorter: '分类机',
  transport: '运输',
  door: '门与机关',
  logic: '逻辑电路',
  utility: '实用装置',
  other: '其他',
}

export const REDSTONE_DESIGN_PURPOSES: RedstoneDesignPurpose[] = [
  'farm',
  'sorter',
  'transport',
  'door',
  'logic',
  'utility',
  'other',
]

/** 难度展示名 */
export const REDSTONE_DIFFICULTY_LABELS: Record<RedstoneDesignDifficulty, string> = {
  easy: '简单',
  medium: '中等',
  hard: '困难',
}

export const REDSTONE_DIFFICULTIES: RedstoneDesignDifficulty[] = ['easy', 'medium', 'hard']

/** 审核状态展示名 */
export const REDSTONE_REVIEW_STATUS_LABELS: Record<RedstoneReviewStatus, string> = {
  pending: '待审核',
  published: '已发布',
  rejected: '已驳回',
}

/** 适用版本展示名 */
export const GAME_EDITION_LABELS: Record<GameEdition, string> = {
  java: 'Java 版',
  bedrock: '基岩版',
  both: 'Java 版 / 基岩版',
}

/** 元件类别徽章配色。 */
export function componentCategoryClass(category: RedstoneComponentCategory): string {
  switch (category) {
    case 'source':
      return 'border-red-500/40 bg-red-500/10 text-red-300'
    case 'transmission':
      return 'border-amber-500/40 bg-amber-500/10 text-amber-300'
    case 'logic':
      return 'border-purple-500/40 bg-purple-500/10 text-purple-300'
    case 'actuator':
      return 'border-grass-500/40 bg-grass-500/10 text-grass-300'
    case 'sensor':
      return 'border-sky-500/40 bg-sky-500/10 text-sky-300'
    default:
      return 'border-border-light bg-ink-700 text-text-muted'
  }
}

/** 设计用途徽章配色。 */
export function designPurposeClass(purpose: RedstoneDesignPurpose): string {
  switch (purpose) {
    case 'farm':
      return 'border-grass-500/40 bg-grass-500/10 text-grass-300'
    case 'sorter':
      return 'border-sky-500/40 bg-sky-500/10 text-sky-300'
    case 'transport':
      return 'border-amber-500/40 bg-amber-500/10 text-amber-300'
    case 'door':
      return 'border-purple-500/40 bg-purple-500/10 text-purple-300'
    case 'logic':
      return 'border-red-500/40 bg-red-500/10 text-red-300'
    default:
      return 'border-border-light bg-ink-700 text-text-muted'
  }
}

/** 难度徽章配色。 */
export function difficultyClass(difficulty: RedstoneDesignDifficulty): string {
  switch (difficulty) {
    case 'easy':
      return 'border-grass-500/40 bg-grass-500/10 text-grass-300'
    case 'hard':
      return 'border-red-500/40 bg-red-500/10 text-red-300'
    default:
      return 'border-amber-500/40 bg-amber-500/10 text-amber-300'
  }
}

/** 材料数量区间展示：固定数量只显示单值。 */
export function formatMaterialCount(min: number, max: number): string {
  return min === max ? String(min) : `${min}–${max}`
}

/** 材料展示文案：`圆石 ×16–64`。 */
export function formatMaterialLabel(material: RedstoneMaterial): string {
  const name = material.name_zh || material.item_id
  return `${name} ×${formatMaterialCount(material.count_min, material.count_max)}`
}

/** 是否为可安全外链的 http(s) 地址（避免生成无效链接）。 */
export function isExternalUrl(url: string | null): boolean {
  return !!url && /^https?:\/\//i.test(url)
}
