// 坐标换算单元测试（需求 13.3：坐标换算结果 / 边界输入）
import { describe, expect, it } from 'vitest'
import {
  convertCoordinate,
  convertWithValidation,
  parseCoordInput,
  portalPair,
  roundCoord,
  routeHint,
  NETHER_RATIO,
} from '@/utils/coordinate'

describe('坐标换算', () => {
  it('验收示例：主世界 (800, 70, -240) -> 下界理论坐标 (100, 70, -30)', () => {
    const result = convertWithValidation(
      { x: '800', y: '70', z: '-240' },
      'overworldToNether',
    )
    expect(result.ok).toBe(true)
    if (result.ok) {
      expect(result.result.x).toBe(100)
      expect(result.result.y).toBe(70)
      expect(result.result.z).toBe(-30)
    }
  })

  it('下界 -> 主世界：X/Z 乘以 8，Y 不变', () => {
    const result = convertWithValidation(
      { x: '100', y: '64', z: '-30' },
      'netherToOverworld',
    )
    expect(result.ok).toBe(true)
    if (result.ok) {
      expect(result.result.x).toBe(800)
      expect(result.result.y).toBe(64)
      expect(result.result.z).toBe(-240)
    }
  })

  it('支持小数输入，不丢失精度', () => {
    const result = convertWithValidation({ x: '100.5', y: '70', z: '0' }, 'overworldToNether')
    expect(result.ok).toBe(true)
    if (result.ok) {
      expect(result.result.x).toBeCloseTo(12.5625, 4)
    }
  })

  it('非法输入（NaN / Infinity / 非数值）报错', () => {
    expect(convertWithValidation({ x: 'abc', y: '70', z: '0' }, 'overworldToNether').ok).toBe(false)
    expect(parseCoordInput({ x: 'Infinity', y: '70', z: '0' }, false).issues.length).toBeGreaterThan(0)
    expect(parseCoordInput({ x: 'NaN', y: '70', z: '0' }, false).issues.length).toBeGreaterThan(0)
  })

  it('空输入（required=true）时提示必填', () => {
    const { issues } = parseCoordInput({ x: '', y: '', z: '' }, true)
    expect(issues.length).toBe(3)
    expect(issues[0].field).toBe('x')
    expect(issues[0].reason).toContain('不能为空')
  })

  it('空输入（required=false）允许，未填的坐标不换算', () => {
    const { value, issues } = parseCoordInput({ x: '', y: '70', z: '' }, false)
    expect(issues.length).toBe(0)
    expect(value.x).toBeNull()
    expect(value.y).toBe(70)
    const result = convertCoordinate(value, 'overworldToNether')
    expect(result.x).toBeNull()
    expect(result.z).toBeNull()
  })

  it('取整建议不覆盖精确结果', () => {
    const result = convertWithValidation({ x: '999', y: '0', z: '0' }, 'overworldToNether')
    expect(result.ok).toBe(true)
    if (result.ok) {
      expect(result.result.x).toBeCloseTo(124.875, 3)
      expect(roundCoord(result.result.x!, 0)).toBe(125)
    }
  })

  it('正数、负数、小数混合', () => {
    const result = convertCoordinate({ x: -800, y: 64.5, z: 0 }, 'overworldToNether')
    expect(result.x).toBe(-100)
    expect(result.y).toBe(64.5)
    expect(result.z).toBe(0)
  })

  it('常量比率正确', () => {
    expect(NETHER_RATIO).toBe(8)
  })
})

describe('传送门理论配对', () => {
  it('主世界坐标给出下界配对（X/Z 除以 8）', () => {
    const pair = portalPair({ x: 800, z: -240 }, 'overworld')
    expect(pair.overworld).toEqual({ x: 800, z: -240 })
    expect(pair.nether).toEqual({ x: 100, z: -30 })
  })

  it('下界坐标给出主世界配对（X/Z 乘以 8）', () => {
    const pair = portalPair({ x: 100, z: -30 }, 'nether')
    expect(pair.overworld).toEqual({ x: 800, z: -240 })
    expect(pair.nether).toEqual({ x: 100, z: -30 })
  })
})

describe('线路规划', () => {
  it('正东方向距离与方位正确', () => {
    const hint = routeHint({ x: 0, z: 0 }, { x: 100, z: 0 })
    expect(hint.distance).toBe(100)
    expect(hint.direction).toBe('东')
    expect(hint.bearing).toBeCloseTo(90, 6)
  })

  it('正北方向（-Z）方位为 0°', () => {
    const hint = routeHint({ x: 0, z: 0 }, { x: 0, z: -64 })
    expect(hint.direction).toBe('北')
    expect(hint.bearing).toBeCloseTo(0, 6)
  })

  it('西南方向（-X、+Z）方位正确', () => {
    const hint = routeHint({ x: 0, z: 0 }, { x: -10, z: 10 })
    expect(hint.direction).toBe('西南')
    expect(hint.bearing).toBeCloseTo(225, 6)
    expect(hint.distance).toBeCloseTo(Math.hypot(10, 10), 6)
  })

  it('相同点距离为 0', () => {
    const hint = routeHint({ x: 5, z: 5 }, { x: 5, z: 5 })
    expect(hint.distance).toBe(0)
  })
})