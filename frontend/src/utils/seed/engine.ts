// ===== 种子本地引擎：纯前端可精确计算的能力 =====
// 用途：不依赖后端即可提供世界种子解析与史莱姆区块扫描。
// 地图/结构（生态群系、要塞、村庄等）依赖后端引擎，通过 api/seed.ts 获取。
import { toWorldSeed, findSlimeChunks, chunkToBlock, isSlimeChunk } from './slime'
import type { LocalSeedResult, SeedVersion } from '@/types/seed'

export interface LocalScanOptions {
  /** 世界种子字符串（已解析） */
  radius: number
  version?: SeedVersion
}

/** 解析种子并计算本地能精确给出的结果 */
export function localSeedAnalysis(
  rawSeed: string,
  opts: LocalScanOptions,
): LocalSeedResult {
  const worldSeed = toWorldSeed(rawSeed)
  const r = Math.max(1, Math.min(64, opts.radius || 8))

  const chunks = findSlimeChunks(worldSeed, {
    minX: -r,
    maxX: r,
    minZ: -r,
    maxZ: r,
  })

  return {
    world_seed: worldSeed.toString(),
    slime_chunks: chunks.map((c) => ({
      type: 'slime_chunk',
      x: chunkToBlock(c.x, c.z).x + 8,
      z: chunkToBlock(c.x, c.z).z + 8,
      label: `史莱姆区块 ${c.x}, ${c.z}`,
    })),
    backend_pending: true,
  }
}

/** 便捷方法：判断区块是否为史莱姆区块（供局部小范围检测） */
export { toWorldSeed, isSlimeChunk }