// ===== Java Random 精确复刻（64 位线性同余）=====
// Minecraft Java 版世界生成使用 java.util.Random。
// 为在浏览器中产生与游戏完全一致的随机序列，这里用 BigInt 复刻其
// 48 位 LCG（multiplier/addend/mask 与 JDK 一致），并提供 nextInt/nextLong。

const MULTIPLIER = 0x5deece66dn
const ADDEND = 0xbn
const MASK = (1n << 48n) - 1n

export class JavaRandom {
  private seed48: bigint

  /** 与 JDK `new Random(seed)` 行为一致：seed = (initial ^ multiplier) & mask */
  constructor(seed: number | bigint) {
    const s = BigInt(seed)
    this.seed48 = (s ^ MULTIPLIER) & MASK
  }

  /** 内部 next(bits)，返回最高 bits 位（JDK Random.next） */
  private next(bits: number): number {
    this.seed48 = (this.seed48 * MULTIPLIER + ADDEND) & MASK
    return Number(this.seed48 >> BigInt(48 - bits))
  }

  /** 等价 JDK nextInt()：32 位带符号随机数 */
  nextInt32(): number {
    // JDK 返回 (int)next(32)，为带符号 32 位
    const u = this.next(32)
    return u | 0
  }

  /** 等价 JDK nextInt(bound)：带拒绝采样，须复刻判断位运算 */
  nextInt(bound: number): number {
    if (bound <= 0) throw new Error('bound must be positive')
    if ((bound & -bound) === bound) {
      // 2 的幂：直接 (bound * next(31)) >> 31
      return (bound * this.next(31)) >> 31
    }
    // 拒绝采样逻辑（与 JDK 一致）
    let bits = 0
    let val = 0
    do {
      bits = this.next(31)
      val = bits % bound
    } while (bits - val + (bound - 1) < 0)
    return val
  }

  /** 等价 JDK nextLong() */
  nextLong(): bigint {
    // JDK: ((long)next(32) << 32) + next(32)
    // 两次 next(32) 均以带符号 32 位参与运算（符号扩展后再移位/相加）
    const high = BigInt(this.next(32) | 0)
    const low = BigInt(this.next(32) | 0)
    return asSigned64((high << 32n) + low)
  }

  /** 等价 JDK nextFloat() */
  nextFloat(): number {
    return this.next(24) / 0x1000000
  }

  /** 等价 JDK nextDouble() */
  nextDouble(): number {
    const high = BigInt(this.next(26)) << 27n
    const low = BigInt(this.next(27))
    return Number(high | low) / 0x20000000000000
  }
}

/** 规约为带符号 64 位整数（BigInt 范围内） */
export function asSigned64(v: bigint): bigint {
  const masked = v & 0xffffffffffffffffn
  return masked >= 0x8000000000000000n ? masked - 0x10000000000000000n : masked
}

/** 无符号 64 位取模（用于保证非负） */
export function u64(v: bigint): bigint {
  return v & 0xffffffffffffffffn
}

/** Java String.hashCode（32 位带符号） */
export function javaStringHash(s: string): number {
  let h = 0
  for (let i = 0; i < s.length; i++) {
    h = (Math.imul(h, 31) + s.charCodeAt(i)) | 0
  }
  return h
}