// ===== 内置工具使用统计上报（TODO §13）=====
//
// 口径（前端、后端与文档必须保持一致）：
//   visit = 用户进入该内置工具页面（路由到达）；
//   use   = 用户执行了工具的核心动作，或复制了工具产出（CopyButton 带 tool 属性时）；
//   fail  = 核心动作执行失败（请求失败 / 解析异常）。
//
// 隐私：只上报「内置工具 slug + 动作」，不携带任何用户标识、路径参数或输入内容。
// 可靠性：best-effort，失败静默；测试环境（MODE=test）不发送，避免单测产生网络噪音。
import { reportToolEvent, type ToolStatAction } from '@/api/stats'
import { BUILTIN_TOOL_ROUTES } from './builtin-tools'

/** 内置工具落地路径 -> slug 反查表 */
const ROUTE_TO_SLUG = new Map(
  Object.entries(BUILTIN_TOOL_ROUTES).map(([slug, route]) => [route, slug]),
)

/** 按路径解析内置工具 slug（非内置工具路径返回 null）。 */
export function builtinToolSlugByPath(path: string): string | null {
  return ROUTE_TO_SLUG.get(path) ?? null
}

/** 发送上报；失败静默，绝不阻塞或影响工具本身。 */
function send(slug: string, action: ToolStatAction): void {
  if (import.meta.env.MODE === 'test') return
  void reportToolEvent(slug, action).catch(() => {})
}

/** 记录一次工具页访问（由路由 afterEach 调用，传入路由路径）。 */
export function reportToolVisit(path: string): void {
  const slug = builtinToolSlugByPath(path)
  if (slug) send(slug, 'visit')
}

/** 记录一次工具成功使用。 */
export function reportToolUse(slug?: string | null): void {
  if (slug) send(slug, 'use')
}

/** 记录一次工具执行失败。 */
export function reportToolFailure(slug?: string | null): void {
  if (slug) send(slug, 'fail')
}
