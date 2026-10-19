// 出生点近似测试（需求 5.1：种子工具 —— 出生点）
import { describe, expect, it } from 'vitest'
import { approximateSpawn, isSpawnBiome } from '@/utils/seed/spawn'
import type { BiomeChunkSample } from '@/types/seed'

describe('出生点近似 · 群系有效性', () => {
  it('陆地群系可作为出生点', () => {
    for (const b of ['plains', 'forest', 'desert', 'taiga', 'savanna', 'ice_spikes']) {
      expect(isSpawnBiome(b)).toBe(true)
    }
  })

  it('水域/海滩/异界群系不可作为出生点', () => {
    for (const b of [
      'ocean',
      'deep_ocean',
      'warm_ocean',
      'frozen_river',
      'river',
      'beach',
      'stone_shore',
      'the_end',
      'nether_wastes',
      'the_void',
    ]) {
      expect(isSpawnBiome(b)).toBe(false)
    }
  })

  it('空群系标识视为无效', () => {
    expect(isSpawnBiome('')).toBe(false)
  })
})

describe('出生点近似 · 最近有效群系', () => {
  it('空采样返回 null', () => {
    expect(approximateSpawn([])).toBeNull()
  })

  it('全部为水域时返回 null', () => {
    const biomes: BiomeChunkSample[] = [
      { cx: 0, cz: 0, biome: 'ocean' },
      { cx: 1, cz: 0, biome: 'river' },
      { cx: 0, cz: 1, biome: 'beach' },
    ]
    expect(approximateSpawn(biomes)).toBeNull()
  })

  it('区块中心换算为方块坐标（cx*16+8）', () => {
    const spawn = approximateSpawn([{ cx: 0, cz: 0, biome: 'plains' }])
    expect(spawn).not.toBeNull()
    expect(spawn).toMatchObject({ x: 8, z: 8, biome: 'plains' })
  })

  it('从原点向外取最近的有效群系', () => {
    const biomes: BiomeChunkSample[] = [
      { cx: 5, cz: 0, biome: 'forest' }, // 距离较远
      { cx: 1, cz: 0, biome: 'plains' }, // 最近
      { cx: -3, cz: -3, biome: 'desert' },
      { cx: 0, cz: 0, biome: 'ocean' }, // 无效
    ]
    const spawn = approximateSpawn(biomes)
    expect(spawn).toMatchObject({ x: 24, z: 8, biome: 'plains' })
    expect(spawn!.distanceBlocks).toBeCloseTo(Math.hypot(24, 8), 6)
  })

  it('成群系命中时按欧氏距离而非区块索引选择', () => {
    const biomes: BiomeChunkSample[] = [
      { cx: 0, cz: 4, biome: 'forest' }, // 8 + 64 -> 约 64.5
      { cx: 3, cz: 2, biome: 'taiga' }, // 远
      { cx: -2, cz: 0, biome: 'savanna' }, // |-32|+8 -> 约 24.9，最近
    ]
    const spawn = approximateSpawn(biomes)
    expect(spawn!.biome).toBe('savanna')
  })
})
