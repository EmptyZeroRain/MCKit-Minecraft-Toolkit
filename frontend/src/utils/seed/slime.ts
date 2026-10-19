// ===== 种子解析与史莱姆区块检测（精确复刻 Java 版算法）=====
// 参考：Minecraft Java 版 isSlimeChunk 实现。种子类型：
// - 数字种子：即世界种子本身（有符号 64 位）
// - 字符串种子：Minecraft 用 Java String.hashCode 再做一次 nextLong
// 史莱姆区块公式（1.8+，普通世界）：
//   rndSeed = worldSeed
//           + (long)(x*x*0x4c1906) + (long)(x*0x5ac0db)
//           + (long)(z*z*0x4307a7) + (long)(z*0x5f24f) ^ 0x3ad8025fL
//   其中 int 运算发生 32 位溢出，之后按有符号 64 位相加
import { JavaRandom, asSigned64, javaStringHash } from './java-random'

/** 32 位有符号截断（模拟 Java int 溢出） */
function asInt32(v: bigint): bigint {
  const masked = v & 0xffffffffn
  return masked >= 0x80000000n ? masked - 0x100000000n : masked
}

/** int 乘法并 32 位截断 */
function mulInt32(a: bigint, b: bigint): bigint {
  return asInt32(a * b)
}

/** 解析用户输入的种子，返回世界种子（有符号 BigInt） */
export function toWorldSeed(input: string): bigint {
  const trimmed = input.trim()
  if (trimmed === '') throw new Error('种子不能为空')
  // 纯数字（可含负号）-> 直接作为世界种子
  if (/^[+-]?\d+$/.test(trimmed)) {
    return BigInt(trimmed)
  }
  // 字符串种子 -> Minecraft 逻辑：hashCode 后取 nextLong
  const hash = javaStringHash(trimmed)
  const rng = new JavaRandom(hash)
  return rng.nextLong()
}

/** 将 world seed 规范化为带符号 64 位 */
export function normalizeSeed(seed: bigint): bigint {
  return asSigned64(seed)
}

export interface SlimeChunk {
  x: number
  z: number
}

/**
 * 判断某个区块是否为史莱姆区块（1.8+ 普通世界）。
 * 精确复刻 Java 的 32 位溢出语义。
 */
export function isSlimeChunk(worldSeed: bigint, chunkX: number, chunkZ: number): boolean {
  const wx = BigInt(chunkX)
  const wz = BigInt(chunkZ)

  // int 运算的乘法链（32 位截断）
  const x2 = mulInt32(wx, wx) // x*x
  const termX = mulInt32(x2, 0x4c1906n) // x*x*0x4c1906
  const termX2 = mulInt32(wx, 0x5ac0dbn) // x*0x5ac0db
  const z2 = mulInt32(wz, wz)
  const termZ = mulInt32(z2, 0x4307a7n) // z*z*0x4307a7
  const termZ2 = mulInt32(wz, 0x5f24fn) // z*0x5f24f

  // 各项以有符号 64 位相加（JDK long 加法），再 XOR
  const combined = asSigned64(worldSeed + termX + termX2 + termZ + termZ2)
  const rndSeed = asSigned64(combined ^ 0x3ad8025fn)

  const rng = new JavaRandom(rndSeed)
  return rng.nextInt(10) === 0
}

/**
 * 扫描某个区块范围内所有史莱姆区块。
 * bounds：以区块为单位 [minX, maxX, minZ, maxZ]（含端点）
 */
export function findSlimeChunks(
  worldSeed: bigint,
  bounds: { minX: number; maxX: number; minZ: number; maxZ: number },
): SlimeChunk[] {
  const out: SlimeChunk[] = []
  for (let x = bounds.minX; x <= bounds.maxX; x++) {
    for (let z = bounds.minZ; z <= bounds.maxZ; z++) {
      if (isSlimeChunk(worldSeed, x, z)) {
        out.push({ x, z })
      }
    }
  }
  return out
}

/** 区块坐标 -> 方块坐标（区块左上/中心） */
export const CHUNK_SIZE = 16

export function chunkToBlock(chunkX: number, chunkZ: number): { x: number; z: number } {
  return { x: chunkX * CHUNK_SIZE, z: chunkZ * CHUNK_SIZE }
}

/** 方块坐标 -> 区块坐标（向下取整，正确处理负坐标） */
export function blockToChunk(blockX: number, blockZ: number): { x: number; z: number } {
  return { x: Math.floor(blockX / CHUNK_SIZE), z: Math.floor(blockZ / CHUNK_SIZE) }
}