// ===== mcsrc 源码 token 工具 =====
// 纯函数：字符偏移 ↔ 行列换算、token 命中判定、成员跳转片段的编解码。
// 与 Monaco / Vue 无关，便于单测覆盖。
import type { McsrcRevealTarget, McsrcToken } from '@/types/mcsrc'

/**
 * 定位字符偏移命中的 token。
 * 要求 `tokens` 按 `start` 升序（Worker 产出时已排序）；多个 token 重叠时取最先出现的一个
 * （与上游 FindDeclaration 的 `tokens.find(...)` 语义一致）。
 */
export function findTokenAtOffset(
  tokens: readonly McsrcToken[] | undefined,
  offset: number,
): McsrcToken | null {
  if (!tokens?.length || offset < 0) return null
  for (const token of tokens) {
    if (token.start > offset) break
    if (offset < token.start + token.length) return token
  }
  return null
}

/** Monaco 位置（1 起行列）→ 字符偏移（换行符按 1 字符计，与 Monaco 模型一致） */
export function offsetAtPosition(
  lines: readonly string[],
  lineNumber: number,
  column: number,
): number {
  let offset = 0
  const last = Math.min(lineNumber - 1, lines.length)
  for (let i = 0; i < last; i++) offset += lines[i].length + 1
  return offset + Math.max(0, column - 1)
}

/** 成员 token → 跳转片段（class / parameter / local token 返回空串） */
export function tokenMemberFragment(token: McsrcToken): string {
  if (token.type === 'field' && token.name) return `field:${token.name}`
  if (token.type === 'method' && token.name) return `method:${token.name}:${token.descriptor ?? ''}`
  return ''
}

/** 跳转片段 → 定位目标；非法片段返回 null */
export function parseMemberFragment(fragment: string | undefined | null): McsrcRevealTarget | null {
  if (!fragment) return null
  const sep = fragment.indexOf(':')
  if (sep < 0) return null
  const kind = fragment.slice(0, sep)
  const rest = fragment.slice(sep + 1)
  if (!rest) return null
  if (kind === 'field') return { type: 'field', name: rest }
  if (kind === 'method') {
    const paramSep = rest.indexOf(':')
    if (paramSep < 0) return { type: 'method', name: rest }
    return { type: 'method', name: rest.slice(0, paramSep), descriptor: rest.slice(paramSep + 1) }
  }
  return null
}

/** 在 token 列表中查找成员声明（用于打开源码后定位） */
export function findDeclarationToken(
  tokens: readonly McsrcToken[] | undefined,
  target: McsrcRevealTarget,
): McsrcToken | null {
  if (!tokens?.length) return null
  for (const token of tokens) {
    if (!token.declaration || token.type !== target.type || token.name !== target.name) continue
    // 字段按名匹配即可；方法在描述符已知时要求一致（重载区分）。
    if (target.type === 'field' || !target.descriptor || token.descriptor === target.descriptor) {
      return token
    }
  }
  return null
}
