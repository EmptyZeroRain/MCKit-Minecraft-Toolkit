// 种子地图工具测试（需求 13.3：搜索/筛选状态、坐标换算）
import { describe, expect, it } from 'vitest'
import {
  blockToPixel,
  pixelToBlock,
  gridStepBlocks,
  mapHalfBlocks,
  structureColor,
  structureLabel,
  STRUCTURE_COLORS,
} from '@/utils/seed/map'

const SIZE = 512

describe('种子地图 · 坐标换算', () => {
  it('世界原点映射到画布中心', () => {
    expect(blockToPixel(0, 0, 256, SIZE)).toEqual({ px: 256, py: 256 })
  })

  it('范围四角映射到画布四角（+Z 向下）', () => {
    expect(blockToPixel(-256, -256, 256, SIZE)).toEqual({ px: 0, py: 0 })
    expect(blockToPixel(256, 256, 256, SIZE)).toEqual({ px: 512, py: 512 })
  })

  it('pixelToBlock 是 blockToPixel 的逆运算', () => {
    for (const [x, z] of [
      [0, 0],
      [128, -64],
      [-256, 256],
      [17, -33],
    ] as const) {
      const { px, py } = blockToPixel(x, z, 256, SIZE)
      const back = pixelToBlock(px, py, 256, SIZE)
      expect(back.x).toBeCloseTo(x, 6)
      expect(back.z).toBeCloseTo(z, 6)
    }
  })
})

describe('种子地图 · 网格与范围', () => {
  it('半径越大网格越粗，且单调不递减', () => {
    const steps = [16, 128, 129, 256, 384, 385, 1024].map(gridStepBlocks)
    for (let i = 1; i < steps.length; i++) {
      expect(steps[i]).toBeGreaterThanOrEqual(steps[i - 1])
    }
    expect(gridStepBlocks(128)).toBe(16)
    expect(gridStepBlocks(129)).toBe(64)
    expect(gridStepBlocks(384)).toBe(64)
    expect(gridStepBlocks(385)).toBe(128)
  })

  it('区块半径 -> 方块半边长为 16 倍，且至少有 1 区块', () => {
    expect(mapHalfBlocks(8)).toBe(128)
    expect(mapHalfBlocks(32)).toBe(512)
    expect(mapHalfBlocks(0)).toBe(16)
    expect(mapHalfBlocks(-5)).toBe(16)
  })
})

describe('种子地图 · 结构配色与名称', () => {
  it('已知类型返回既定配色', () => {
    expect(structureColor('slime_chunk')).toBe(STRUCTURE_COLORS.slime_chunk)
    expect(structureColor('village')).toBe(STRUCTURE_COLORS.village)
  })

  it('未知类型回退到中性色而非透明', () => {
    expect(structureColor('not_a_structure')).toBe('#9aa9c4')
  })

  it('已知类型有中文名，未知类型原样返回', () => {
    expect(structureLabel('slime_chunk')).toBe('史莱姆区块')
    expect(structureLabel('stronghold')).toBe('要塞')
    expect(structureLabel('unknown_thing')).toBe('unknown_thing')
  })
})
