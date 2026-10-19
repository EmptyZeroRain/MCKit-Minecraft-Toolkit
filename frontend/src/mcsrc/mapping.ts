// ===== 映射重命名匹配 =====
//
// 服务端按「版本 + jar 内类名」反查映射（见 /mcsrc/classes/mapping），返回该类的
// 类级名与全部成员映射。这里把成员映射与结构视图里的字段 / 方法对应起来。
//
// 匹配原则（内容准确性优先）：
//   1. 按混淆名（obf_name）匹配 —— jar 内的成员名就是混淆名；
//   2. 若映射提供混淆描述符（Tiny / TSRG），用描述符精确区分重载；
//   3. 无法唯一确定可读名时**返回空串**（宁可不展示，也不给出可能错误的映射名）。
import type { McsrcClassMapping, McsrcMappingMember } from '@/types/mcsrc'

/** 映射成员的可读名：优先可读名，回退中间名。 */
function memberName(m: McsrcMappingMember): string {
  return m.named_name || m.mid_name || ''
}

/** 类映射的可读名：优先可读名 → 中间名 → 混淆名。 */
export function classMappingName(mapping: McsrcClassMapping | null | undefined): string {
  const c = mapping?.class
  if (!c) return ''
  return c.named_name || c.mid_name || c.obf_name || ''
}

/**
 * 成员映射名：按混淆名匹配，优先用混淆描述符精确区分重载。
 * 无法唯一确定时返回空串。
 */
export function memberMappingName(
  mapping: McsrcClassMapping | null | undefined,
  kind: 'method' | 'field',
  name: string,
  descriptor: string,
): string {
  const candidates = (mapping?.members ?? []).filter((m) => m.kind === kind && m.obf_name === name)
  if (!candidates.length) return ''

  // 1) 描述符精确匹配（仅 Tiny / TSRG 提供混淆描述符）。
  const byDesc = candidates.filter((m) => m.obf_desc && m.obf_desc === descriptor)
  if (byDesc.length) {
    const names = new Set(byDesc.map(memberName).filter(Boolean))
    return names.size === 1 ? [...names][0] : ''
  }

  // 2) 回退：仅当所有同名候选给出同一个可读名时才展示。
  const names = new Set(candidates.map(memberName).filter(Boolean))
  return names.size === 1 ? [...names][0] : ''
}

/** 映射来源描述：如 `mojang · mojang-1.21.4`。 */
export function mappingSourceLabel(mapping: McsrcClassMapping | null | undefined): string {
  if (!mapping?.available) return ''
  return [mapping.mapping_type, mapping.set_key].filter(Boolean).join(' · ')
}
