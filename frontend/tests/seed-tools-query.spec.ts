// 种子与结构工具：URL 状态编解码测试（TODO §14）
// 保证「默认状态 ⇔ 无参数」双向一致，且非法值不会污染分享链接。
import { describe, expect, it } from 'vitest'
import {
  SEED_TOOLS_DEFAULTS,
  buildSeedToolsQuery,
  readSeedToolsQuery,
} from '@/utils/seed-tools-query'

describe('seed-tools URL 状态', () => {
  it('空 query 还原为默认状态', () => {
    expect(readSeedToolsQuery({})).toEqual(SEED_TOOLS_DEFAULTS)
  })

  it('默认状态编码为「无参数」链接', () => {
    expect(buildSeedToolsQuery(SEED_TOOLS_DEFAULTS)).toEqual({})
  })

  it('非默认值可完整往返（含各子表单）', () => {
    const query = {
      v: '1.16-1.17',
      parse: 'my world',
      gen: '10',
      stSeed: '-1',
      stType: 'stronghold',
      stRx: '-2',
      stRz: '3',
      stCx: '2',
      stCz: '4',
      bioSeed: '42',
      bioX: '100',
      bioZ: '-100',
      shSeed: '7',
      shCount: '5',
      slSeed: '9',
      slMinX: '-8',
      slMinZ: '-8',
      slMaxX: '8',
      slMaxZ: '8',
      fdType: 'ocean_monument',
      fdCx: '64',
      fdCz: '-64',
      fdRadius: '512',
      fdLimit: '3',
    }
    const state = readSeedToolsQuery(query)
    expect(state.version).toBe('1.16-1.17')
    expect(state.parseInput).toBe('my world')
    expect(state.genCount).toBe(10)
    expect(state.structures).toEqual({
      seed: '-1',
      type: 'stronghold',
      region_x: -2,
      region_z: 3,
      count_x: 2,
      count_z: 4,
    })
    expect(state.biome).toEqual({ seed: '42', x: 100, z: -100 })
    expect(state.stronghold).toEqual({ seed: '7', count: 5 })
    expect(state.slime).toEqual({
      seed: '9',
      min_chunk_x: -8,
      min_chunk_z: -8,
      max_chunk_x: 8,
      max_chunk_z: 8,
    })
    expect(state.find).toEqual({ type: 'ocean_monument', center_x: 64, center_z: -64, radius: 512, limit: 3 })

    // 再编码应得到等价参数集合（不同顺序不影响语义）
    expect(buildSeedToolsQuery(state)).toEqual(query)
  })

  it('非法版本与非法数字回退默认值（避免坏链接导致接口 422）', () => {
    const state = readSeedToolsQuery({ v: 'not-a-version', gen: 'abc', stRx: 'NaN' })
    expect(state.version).toBe(SEED_TOOLS_DEFAULTS.version)
    expect(state.genCount).toBe(SEED_TOOLS_DEFAULTS.genCount)
    expect(state.structures.region_x).toBe(SEED_TOOLS_DEFAULTS.structures.region_x)
  })

  it('非法结构类型回退默认值，合法 snake_case 保留', () => {
    const bad = readSeedToolsQuery({ stType: 'DROP TABLE', fdType: '../../etc/passwd' })
    expect(bad.structures.type).toBe(SEED_TOOLS_DEFAULTS.structures.type)
    expect(bad.find.type).toBe(SEED_TOOLS_DEFAULTS.find.type)

    const ok = readSeedToolsQuery({ stType: 'trail_ruins' })
    expect(ok.structures.type).toBe('trail_ruins')
  })

  it('0 值不会被当作「空值」丢弃（坐标 0 是合法输入）', () => {
    const state = readSeedToolsQuery({ bioX: '0', bioSeed: '1' })
    expect(state.biome).toEqual({ seed: '1', x: 0, z: 0 })
    // x/z 为 0 属于默认值，因此链接中只保留 seed
    expect(buildSeedToolsQuery(state)).toEqual({ bioSeed: '1' })
  })
})
