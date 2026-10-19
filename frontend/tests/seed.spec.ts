// 种子引擎单元测试（需求 13.1：种子解析、史莱姆区块算法确定性）
// 参考值全部由 JDK 21 `java.util.Random` 与 Java 版史莱姆区块算法实测生成。
import { describe, expect, it } from 'vitest'
import { JavaRandom, javaStringHash, asSigned64 } from '@/utils/seed/java-random'
import { toWorldSeed, isSlimeChunk, findSlimeChunks, blockToChunk } from '@/utils/seed/slime'
import { localSeedAnalysis } from '@/utils/seed/engine'

describe('Java Random 复刻', () => {
  it('nextLong 与 JDK 一致（new Random(0).nextLong()）', () => {
    expect(new JavaRandom(0n).nextLong()).toBe(-4962768465676381896n)
  })

  it('nextInt(bound) 确定性：seed=12345 连续 nextInt(10) 与 JDK 一致', () => {
    const rng = new JavaRandom(12345n)
    const expected = [1, 0, 1, 8, 5, 4, 5, 2, 1, 9]
    const actual = Array.from({ length: 10 }, () => rng.nextInt(10))
    expect(actual).toEqual(expected)
  })

  it('nextInt32 与 JDK 一致（new Random(42).nextInt()）', () => {
    expect(new JavaRandom(42n).nextInt32()).toBe(-1170105035)
  })

  it('javaStringHash 与 JDK String.hashCode 一致', () => {
    expect(javaStringHash('hello')).toBe(99162322)
    expect(javaStringHash('Minecraft')).toBe(-1595926131)
    expect(javaStringHash('12345')).toBe(46792755)
  })
})

describe('世界种子解析', () => {
  it('数字种子直接使用', () => {
    expect(toWorldSeed('123456789')).toBe(123456789n)
    expect(toWorldSeed('-42')).toBe(-42n)
  })

  it('字符串种子按 Minecraft 规则转换（new Random(hash).nextLong()）', () => {
    expect(toWorldSeed('hello')).toBe(6589081269008824630n)
  })

  it('空种子报错', () => {
    expect(() => toWorldSeed('  ')).toThrow()
  })
})

describe('史莱姆区块算法', () => {
  it('与 Java 版实测一致：种子 12345 半径 4 的 9 个史莱姆区块', () => {
    const seed = toWorldSeed('12345')
    const chunks = findSlimeChunks(seed, { minX: -4, maxX: 4, minZ: -4, maxZ: 4 })
    const actual = chunks.map((c) => `${c.x},${c.z}`)
    expect(actual).toEqual([
      '-4,0',
      '-2,-4',
      '-2,1',
      '-2,4',
      '-1,2',
      '0,-2',
      '3,0',
      '4,-3',
      '4,1',
    ])
  })

  it('已知参考点与 Java 实测一致', () => {
    expect(isSlimeChunk(toWorldSeed('2544892146'), 96, -32)).toBe(true)
    expect(isSlimeChunk(toWorldSeed('2544892146'), 0, 0)).toBe(false)
    expect(isSlimeChunk(toWorldSeed('12345'), 0, 0)).toBe(false)
  })

  it('确定性：同一种子扫描结果稳定', () => {
    const seed = toWorldSeed('minecraft')
    const a = findSlimeChunks(seed, { minX: -4, maxX: 4, minZ: -4, maxZ: 4 })
    const b = findSlimeChunks(seed, { minX: -4, maxX: 4, minZ: -4, maxZ: 4 })
    expect(a).toEqual(b)
  })

  it('区块坐标边界：负坐标向下取整', () => {
    expect(blockToChunk(15, 15)).toEqual({ x: 0, z: 0 })
    expect(blockToChunk(0, 0)).toEqual({ x: 0, z: 0 })
    expect(blockToChunk(-1, -1)).toEqual({ x: -1, z: -1 })
    expect(blockToChunk(-16, -16)).toEqual({ x: -1, z: -1 })
  })

  it('asSigned64 边界规约', () => {
    expect(asSigned64(0xffffffffffffffffn)).toBe(-1n)
    expect(asSigned64(0x8000000000000000n)).toBe(-9223372036854775808n)
    expect(asSigned64(10n)).toBe(10n)
  })
})

describe('本地引擎集成', () => {
  it('localSeedAnalysis 返回解析种子与史莱姆区块', () => {
    const res = localSeedAnalysis('12345', { radius: 4 })
    expect(res.world_seed).toBe('12345')
    expect(res.slime_chunks.length).toBe(9)
    expect(res.backend_pending).toBe(true)
    for (const s of res.slime_chunks) {
      expect(typeof s.x).toBe('number')
      expect(typeof s.z).toBe('number')
      expect(s.type).toBe('slime_chunk')
    }
  })
})