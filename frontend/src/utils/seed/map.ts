// ===== 种子地图：坐标换算与配色（纯函数，独立于视图便于单测）=====
import type { SeedStructureType } from '@/types/seed'

/** 结构类型配色（地图标记与图例共用） */
export const STRUCTURE_COLORS: Record<SeedStructureType, string> = {
  stronghold: '#c084fc',
  village: '#fbbf24',
  ancient_city: '#22d3ee',
  mansion: '#f472b6',
  monument: '#38bdf8',
  outpost: '#fb923c',
  ruined_portal: '#a78bfa',
  end_city: '#facc15',
  desert_pyramid: '#f59e0b',
  jungle_pyramid: '#34d399',
  igloo: '#bae6fd',
  swamp_hut: '#84cc16',
  ocean_ruin: '#2dd4bf',
  shipwreck: '#60a5fa',
  trail_ruins: '#d8b4fe',
  trial_chambers: '#f87171',
  buried_treasure: '#fde68a',
  spawn: '#4ade80',
  slime_chunk: '#7cd15f',
}

const FALLBACK_COLOR = '#9aa9c4'

/** 结构类型 -> 颜色；未知类型给中性色，避免出现透明/黑色标记 */
export function structureColor(type: string): string {
  return STRUCTURE_COLORS[type as SeedStructureType] ?? FALLBACK_COLOR
}

/** 结构类型中文名 */
export const STRUCTURE_LABELS: Record<SeedStructureType, string> = {
  stronghold: '要塞',
  village: '村庄',
  ancient_city: '远古城市',
  mansion: '林地府邸',
  monument: '海底神殿',
  outpost: '掠夺者前哨站',
  ruined_portal: '废弃传送门',
  end_city: '末地城',
  desert_pyramid: '沙漠神殿',
  jungle_pyramid: '丛林神庙',
  igloo: '雪屋',
  swamp_hut: '女巫小屋',
  ocean_ruin: '海底废墟',
  shipwreck: '沉船',
  trail_ruins: '古迹废墟',
  trial_chambers: '试炼密室',
  buried_treasure: '埋藏的宝藏',
  spawn: '出生点',
  slime_chunk: '史莱姆区块',
}

export function structureLabel(type: string): string {
  return STRUCTURE_LABELS[type as SeedStructureType] ?? type
}

/**
 * 方块坐标 -> 画布像素。
 * 地图以世界原点为中心，half 为半边长（方块），size 为画布边长（像素）。
 * +X 向右、+Z 向下（与游戏俯视图一致）。
 */
export function blockToPixel(
  x: number,
  z: number,
  half: number,
  size: number,
): { px: number; py: number } {
  const scale = size / (half * 2)
  return { px: size / 2 + x * scale, py: size / 2 + z * scale }
}

/** 画布像素 -> 方块坐标（blockToPixel 的逆运算） */
export function pixelToBlock(
  px: number,
  py: number,
  half: number,
  size: number,
): { x: number; z: number } {
  const scale = size / (half * 2)
  return { x: (px - size / 2) / scale, z: (py - size / 2) / scale }
}

/**
 * 网格线间隔（方块）：半径越大网格越粗，避免线条糊成一片。
 */
export function gridStepBlocks(half: number): number {
  if (half <= 128) return 16
  if (half <= 384) return 64
  return 128
}

/** 给定半径（区块）下的地图半边长（方块） */
export function mapHalfBlocks(radiusChunks: number): number {
  return Math.max(1, radiusChunks) * 16
}
