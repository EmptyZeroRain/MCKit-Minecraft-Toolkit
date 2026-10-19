// ===== 出生点近似（需求 §5.1 种子工具）=====
//
// 说明：Minecraft 真实出生点由世界生成器在原点附近螺旋搜索「首个满足条件的
// 有效出生群系」并取其地表高度决定，需要完整高度图/地表生成器。
// 此处为**近似**：依据后端 1.18+ 群系采样结果（BiomeChunkSample），
// 从世界原点向外取最近的有效出生群系区块中心作为估算出生点。
// 结果可能偏移数十方块，仅供导航参考，非精确值。
import type { BiomeChunkSample } from '@/types/seed'

/**
 * 不可作为出生点的群系（海洋/河流/海滩/虚空/下界/末地等）。
 * 命名与后端 mcbiome.BiomeName 保持一致（cubiomes 词表）。
 */
const INVALID_SPAWN_BIOMES = new Set<string>([
  // 海洋
  'ocean',
  'deep_ocean',
  'warm_ocean',
  'lukewarm_ocean',
  'cold_ocean',
  'frozen_ocean',
  'deep_warm_ocean',
  'deep_lukewarm_ocean',
  'deep_cold_ocean',
  'deep_frozen_ocean',
  // 河流
  'river',
  'frozen_river',
  // 海滩 / 石岸
  'beach',
  'snowy_beach',
  'stone_shore',
  'mushroom_field_shore',
  // 虚空 / 下界 / 末地
  'the_void',
  'nether_wastes',
  'soul_sand_valley',
  'crimson_forest',
  'warped_forest',
  'basalt_deltas',
  'the_end',
  'small_end_islands',
  'end_midlands',
  'end_highlands',
  'end_barrens',
])

/** 判断某群系是否可作为出生点（排除水域/海滩/异界等）。 */
export function isSpawnBiome(biome: string): boolean {
  return biome !== '' && !INVALID_SPAWN_BIOMES.has(biome)
}

/** 近似出生点结果。 */
export interface ApproxSpawn {
  /** 方块坐标（区块中心） */
  x: number
  z: number
  /** 命中群系标识 */
  biome: string
  /** 距世界原点的方块距离 */
  distanceBlocks: number
}

/** 单区块边长（方块）。 */
const CHUNK_SIZE = 16

/**
 * 从世界原点向外，取最近的有效出生群系区块中心（方块坐标）。
 * 无有效群系时返回 null。
 */
export function approximateSpawn(biomes: BiomeChunkSample[]): ApproxSpawn | null {
  let best: { x: number; z: number; biome: string; d2: number } | null = null
  for (const b of biomes) {
    if (!isSpawnBiome(b.biome)) continue
    const x = b.cx * CHUNK_SIZE + CHUNK_SIZE / 2
    const z = b.cz * CHUNK_SIZE + CHUNK_SIZE / 2
    const d2 = x * x + z * z
    if (!best || d2 < best.d2) {
      best = { x, z, biome: b.biome, d2 }
    }
  }
  if (!best) return null
  return {
    x: best.x,
    z: best.z,
    biome: best.biome,
    distanceBlocks: Math.sqrt(best.d2),
  }
}
