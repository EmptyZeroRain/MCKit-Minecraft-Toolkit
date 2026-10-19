// ===== 区块文件（.mca）校验（纯函数，便于单测）=====
// 文件名必须形如 r.X.Z.mca —— 后端据文件名确定区域坐标，
// 因此前端先做一次拦截，给出比 422 更友好的提示。

/** 单次上传上限，与后端 maxRegionUpload 对齐（64MiB） */
export const MAX_REGION_UPLOAD_BYTES = 64 * 1024 * 1024

/** level.dat 上传上限，与后端 maxLevelUpload 对齐（64MiB） */
export const MAX_LEVEL_UPLOAD_BYTES = 64 * 1024 * 1024

const REGION_NAME_RE = /^r\.(-?\d+)\.(-?\d+)\.mca$/i
const LEVEL_NAME_RE = /^level\.dat$/i

/** 待分析文件的类型：region=.mca 区域文件；level=level.dat 世界存档 */
export type SaveFileKind = 'region' | 'level'

/** 按文件名判定文件类型；无法识别返回 null */
export function classifySaveFile(name: string): SaveFileKind | null {
  if (REGION_NAME_RE.test(name)) return 'region'
  if (LEVEL_NAME_RE.test(name)) return 'level'
  return null
}

/** 从文件名解析区域坐标；非法返回 null */
export function parseRegionFileName(name: string): { x: number; z: number } | null {
  const m = REGION_NAME_RE.exec(name)
  if (!m) return null
  const x = Number(m[1])
  const z = Number(m[2])
  if (!Number.isSafeInteger(x) || !Number.isSafeInteger(z)) return null
  return { x, z }
}

/** 字节数 -> 人类可读（MiB） */
export function formatMiB(bytes: number): string {
  return `${(bytes / 1024 / 1024).toFixed(2)} MiB`
}

/**
 * 校验待上传的区域文件。
 * 通过返回 null，否则返回可直接展示给用户的错误文案。
 */
export function validateRegionFile(name: string, size: number): string | null {
  if (!parseRegionFileName(name)) {
    return '文件名需形如 r.0.0.mca（后端据此确定区域坐标）'
  }
  if (size > MAX_REGION_UPLOAD_BYTES) {
    return `文件过大（${formatMiB(size)}），上限 64 MiB`
  }
  return null
}

/**
 * 校验待上传的 level.dat。
 * 通过返回 null，否则返回可直接展示给用户的错误文案。
 */
export function validateLevelFile(name: string, size: number): string | null {
  if (!LEVEL_NAME_RE.test(name)) {
    return '文件名需为 level.dat'
  }
  if (size > MAX_LEVEL_UPLOAD_BYTES) {
    return `文件过大（${formatMiB(size)}），上限 64 MiB`
  }
  return null
}
