// ===== mcsrc 跳转目标解析 =====
// 复刻上游 FindDeclaration 的语义：成员引用先看引用点所属类，再沿祖先链向上找真正声明该成员的类
// （引用点看到的类可能是子类，成员实际声明在父类）。
//
// 数据来源是后端**已建好的类索引**（classes/detail + classes/hierarchy），
// 因此解析跳转目标不需要重新下载或反编译 —— 只有确定目标类之后才需要反编译该类的源码。
import { getClassDetail, getClassHierarchy } from '@/api/mcsrc'
import type { McsrcRevealTarget, McsrcSide, McsrcTokenType } from '@/types/mcsrc'

/** 待解析的引用 */
export interface DeclarationQuery {
  /** 点号全限定类名（引用点所属类） */
  className: string
  type: McsrcTokenType
  /** 成员名（field / method token 必填） */
  name?: string
  /** JVM 描述符 */
  descriptor?: string
}

export interface DeclarationResult {
  /** 成员真正的声明类；类 token 或无法定位时即引用点所属类 */
  className: string
  /** 需要在目标类源码中定位的成员 */
  reveal: McsrcRevealTarget | null
}

// 每个类声明的成员集合缓存：key = `${version}|${side}|${className}`
const memberCache = new Map<string, Set<string>>()

function memberKey(type: 'field' | 'method', name: string, descriptor?: string): string {
  return type === 'method' ? `m:${name}${descriptor ?? ''}` : `f:${name}`
}

/** 取某类声明的成员集合；类不在 jar 内（索引缺失）时返回空集合 */
async function declaredMembers(
  version: string,
  side: McsrcSide,
  className: string,
): Promise<Set<string>> {
  const key = `${version}|${side}|${className}`
  const cached = memberCache.get(key)
  if (cached) return cached
  const set = new Set<string>()
  try {
    const detail = await getClassDetail(version, className, side)
    for (const field of detail.fields ?? []) set.add(memberKey('field', field.name, field.descriptor))
    for (const method of detail.methods ?? []) set.add(memberKey('method', method.name, method.descriptor))
  } catch {
    // 类不在 jar 内或索引缺失：视为「不声明」，调用方继续沿祖先链向上找。
  }
  memberCache.set(key, set)
  return set
}

/**
 * 解析跳转目标。
 * - class token：目标即该类自身；
 * - field / method token：先看引用点所属类，未命中则沿祖先链（父类 → 更上层）查找声明类。
 */
export async function resolveDeclaration(
  version: string,
  side: McsrcSide,
  query: DeclarationQuery,
): Promise<DeclarationResult> {
  if ((query.type !== 'field' && query.type !== 'method') || !query.name) {
    return { className: query.className, reveal: null }
  }
  const reveal: McsrcRevealTarget = {
    type: query.type,
    name: query.name,
    descriptor: query.descriptor,
  }
  const want = memberKey(query.type, query.name, query.descriptor)
  if ((await declaredMembers(version, side, query.className)).has(want)) {
    return { className: query.className, reveal }
  }
  let ancestors: string[] = []
  try {
    ancestors = (await getClassHierarchy(version, query.className, side)).ancestors ?? []
  } catch {
    ancestors = []
  }
  for (const ancestor of ancestors) {
    if ((await declaredMembers(version, side, ancestor)).has(want)) {
      return { className: ancestor, reveal }
    }
  }
  // 全部未命中（例如声明在 jar 外的第三方库）：退回引用点所属类，至少能打开源码。
  return { className: query.className, reveal }
}

/** 清空成员缓存（版本被重新准备 / 侧别切换后调用） */
export function clearDeclarationCache(): void {
  memberCache.clear()
}
