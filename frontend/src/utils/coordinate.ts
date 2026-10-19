// ===== 坐标换算（需求 5.3）=====
// 主世界 <-> 下界：X / Z 除以 8（主世界→下界），乘以 8（下界→主世界）。
// Y 坐标不参与换算。支持负数和小数。核心逻辑务必保持纯函数便于测试。

export type CoordDirection = 'overworldToNether' | 'netherToOverworld'

export const NETHER_RATIO = 8

export interface CoordInput {
  x: number | null
  y: number | null
  z: number | null
}

export interface CoordResult {
  x: number | null
  y: number | null
  z: number | null
}

export interface ValidationIssue {
  field: 'x' | 'y' | 'z'
  reason: string
}

/**
 * 解析坐标输入。空值允许（视为不参与换算，可单独填写）；
 * NaN / Infinity / 非数值字符串报错。
 * 说明：HTML number input 已拦截大部分文本，这里兜底处理粘贴与程序化输入。
 */
export function parseCoordInput(
  raw: { x: string; y: string; z: string },
  required: boolean,
): { value: CoordInput; issues: ValidationIssue[] } {
  const issues: ValidationIssue[] = []

  const parse = (field: keyof CoordInput, label: string): number | null => {
    const text = raw[field].trim()
    if (text === '') {
      if (required) {
        issues.push({ field, reason: `${label}坐标不能为空` })
      }
      return null
    }
    const num = Number(text)
    if (!Number.isFinite(num)) {
      issues.push({ field, reason: `${label}坐标必须是有效数值` })
      return null
    }
    return num
  }

  const x = parse('x', 'X')
  const y = parse('y', 'Y')
  const z = parse('z', 'Z')

  return { value: { x, y, z }, issues }
}

/**
 * 执行坐标换算。
 * - 为空的坐标保持 null（未填写则不换算）
 * - Y 原样保留
 * - 返回精确计算结果；取整展示由展示层负责，不覆盖精确值（需求 5.3）
 */
export function convertCoordinate(
  input: CoordInput,
  direction: CoordDirection,
): CoordResult {
  const factor = direction === 'overworldToNether' ? 1 / NETHER_RATIO : NETHER_RATIO
  return {
    x: input.x !== null ? input.x * factor : null,
    y: input.y,
    z: input.z !== null ? input.z * factor : null,
  }
}

/**
 * 四舍五入到指定小数位（避免浮点误差，如 100.00000001）。
 */
export function roundCoord(value: number, decimalPlaces = 2): number {
  if (!Number.isFinite(value)) return value
  const factor = 10 ** decimalPlaces
  return Math.round(value * factor) / factor
}

/**
 * 完整的换算：输入 -> 校验 -> 换算 -> 保留 2 位小数（原始精度仍在下方展示）。
 * 返回换算结果与可选的取整建议坐标（不覆盖精确结果）。
 */
export function convertWithValidation(
  raw: { x: string; y: string; z: string },
  direction: CoordDirection,
  required = false,
): { ok: true; result: CoordResult; issues: [] } | { ok: false; issues: ValidationIssue[] } {
  const { value, issues } = parseCoordInput(raw, required)
  if (issues.length > 0) {
    return { ok: false, issues }
  }
  const result = convertCoordinate(value, direction)
  return { ok: true, result, issues: [] }
}

// ===== 传送门配对与路线提示（todo §10 第 4 项）=====
// 说明：主世界与下界按 1:8 对应，这是游戏机制中的换算关系；
// 但实际传送门连接会在目标坐标附近搜索已有传送门，可能产生偏移，
// 且受空间占用与 Y 高度影响，因此这里的配对为「理论对应坐标」。

/** 水平面坐标（方块）。 */
export interface HorizontalPos {
  x: number
  z: number
}

/** 传送门理论配对坐标。 */
export interface PortalPair {
  overworld: HorizontalPos
  nether: HorizontalPos
}

/**
 * 由某一维度的坐标给出主世界/下界的理论配对坐标。
 * dimension 指输入坐标所在维度。
 */
export function portalPair(point: HorizontalPos, dimension: 'overworld' | 'nether'): PortalPair {
  if (dimension === 'overworld') {
    return {
      overworld: { x: point.x, z: point.z },
      nether: { x: point.x / NETHER_RATIO, z: point.z / NETHER_RATIO },
    }
  }
  return {
    overworld: { x: point.x * NETHER_RATIO, z: point.z * NETHER_RATIO },
    nether: { x: point.x, z: point.z },
  }
}

/** 航线（水平面）提示。 */
export interface RouteHint {
  /** 水平直线距离（方块） */
  distance: number
  /** 方位角（度），0=北(-Z)，顺时针 90=东(+X) */
  bearing: number
  /** 八方位中文标签 */
  direction: string
  dx: number
  dz: number
}

const DIRECTIONS = ['北', '东北', '东', '东南', '南', '西南', '西', '西北'] as const

/**
 * 计算两点间的水平距离与方位（供线路规划参考）。
 * Minecraft 坐标：+X 为东、-Z 为北。
 */
export function routeHint(from: HorizontalPos, to: HorizontalPos): RouteHint {
  const dx = to.x - from.x
  const dz = to.z - from.z
  const distance = Math.hypot(dx, dz)
  // atan2(dx, -dz)：-Z（北）为 0°，顺时针增大。
  let bearing = (Math.atan2(dx, -dz) * 180) / Math.PI
  if (bearing < 0) bearing += 360
  const index = Math.round(bearing / 45) % 8
  return { distance, bearing, direction: DIRECTIONS[index], dx, dz }
}

/** 展示格式化：保留 null 时间戳为 '—' */
export function formatCoordNumber(n: number): string {
  return Number.isInteger(n) ? String(n) : String(roundCoord(n, 4))
}