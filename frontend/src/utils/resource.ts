// ===== 资源元数据展示辅助（TODO §6 资源兼容性与来源信息）=====
import { LINK_STATUS_LABELS, type LinkStatus, type LoaderType } from '@/types/common'
import { ALL_VERSIONS } from '@/types/version'
import type {
  Resource,
  ResourceCompatibility,
  ResourceDependency,
  ResourceDependencyType,
  ResourceRelease,
} from '@/types/resource'

/** 依赖分组的展示顺序：前置 → 依赖 → 冲突（与安装顺序一致）。 */
export const DEPENDENCY_TYPE_ORDER: ResourceDependencyType[] = [
  'prerequisite',
  'dependency',
  'conflict',
]

export const DEPENDENCY_TYPE_LABELS: Record<ResourceDependencyType, string> = {
  prerequisite: '前置 Mod',
  dependency: '依赖',
  conflict: '已知冲突',
}

export interface DependencyGroup {
  type: ResourceDependencyType
  label: string
  items: ResourceDependency[]
}

/** 规范化依赖类型：缺省或非法值一律视为 dependency，避免界面出现空分组。 */
export function normalizeDependencyType(type?: string): ResourceDependencyType {
  if (type === 'prerequisite' || type === 'conflict') return type
  return 'dependency'
}

/**
 * 按类型分组依赖项。
 * 空分组不返回，避免渲染空标题；组内保持原始顺序。
 */
export function dependencyGroups(deps?: ResourceDependency[] | null): DependencyGroup[] {
  if (!deps?.length) return []
  const buckets = new Map<ResourceDependencyType, ResourceDependency[]>()
  for (const dep of deps) {
    const type = normalizeDependencyType(dep.type)
    const list = buckets.get(type)
    if (list) {
      list.push(dep)
    } else {
      buckets.set(type, [dep])
    }
  }
  return DEPENDENCY_TYPE_ORDER.filter((t) => buckets.has(t)).map((t) => ({
    type: t,
    label: DEPENDENCY_TYPE_LABELS[t],
    items: buckets.get(t) as ResourceDependency[],
  }))
}

export interface CompatibilitySummary {
  verified: boolean
  /** 具体版本声明（不含通配 `all`，通配由 allVersions 表示） */
  gameVersions: string[]
  loaderTypes: LoaderType[]
  /** 是否声明了「所有版本」通配 */
  allVersions: boolean
  /** 结论依据文案（仅 verified 时有意义） */
  sourceLabel: string
  /** 人工核验备注 */
  note: string
  /** 未验证说明 */
  hint: string
}

const SOURCE_LABELS: Record<string, string> = {
  resource: '资源级字段',
  releases: '发布版本',
}

const DEFAULT_UNVERIFIED_HINT = '该资源未提供适用版本或加载器信息，兼容性尚未验证。'

/** 从版本声明中拆出「具体版本」与「是否通配」 */
function splitVersions(versions: string[] | null | undefined): {
  specific: string[]
  all: boolean
} {
  const specific: string[] = []
  let all = false
  for (const v of versions ?? []) {
    if (v === ALL_VERSIONS) all = true
    else specific.push(v)
  }
  return { specific, all }
}

/**
 * 兼容性结论（防御式）。
 *
 * 接口未返回 `compatibility` 时（例如旧数据或字段缺失）按「未验证」处理，
 * 只用资源级字段作为兜底展示，绝不据此断言兼容。
 */
export function compatibilitySummary(
  resource: Resource,
  compatibility?: ResourceCompatibility | null,
): CompatibilitySummary {
  if (compatibility) {
    const { specific, all } = splitVersions(compatibility.game_versions)
    return {
      verified: compatibility.verified === true,
      gameVersions: specific,
      loaderTypes: compatibility.loader_types ?? [],
      allVersions: compatibility.all_versions === true || all,
      sourceLabel: compatibilitySourceLabel(compatibility.sources),
      note: compatibility.note ?? '',
      hint: compatibility.unverified_hint || DEFAULT_UNVERIFIED_HINT,
    }
  }
  const { specific, all } = splitVersions(resource.supported_versions)
  return {
    verified: false,
    gameVersions: specific,
    loaderTypes: resource.loader_types ?? [],
    allVersions: all,
    sourceLabel: '',
    note: resource.compatibility_note ?? '',
    hint: DEFAULT_UNVERIFIED_HINT,
  }
}

/** 结论依据文案：把来源标识翻译为可读文案。 */
export function compatibilitySourceLabel(sources?: string[] | null): string {
  if (!sources?.length) return ''
  return sources.map((s) => SOURCE_LABELS[s] ?? s).join(' + ')
}

export interface LinkStatusSummary {
  label: string
  /** 展示语气，仅描述客观状态，不给出「安全」结论 */
  tone: 'ok' | 'warn' | 'bad' | 'muted'
}

/** 外链检查状态展示（TODO §6：客观呈现检查结果，不额外推断）。 */
export function linkStatusSummary(status: LinkStatus | undefined | null): LinkStatusSummary {
  switch (status) {
    case 'ok':
      return { label: LINK_STATUS_LABELS.ok, tone: 'ok' }
    case 'redirected':
      return { label: LINK_STATUS_LABELS.redirected, tone: 'warn' }
    case 'broken':
      return { label: LINK_STATUS_LABELS.broken, tone: 'bad' }
    case 'blocked':
    case 'error':
      return { label: LINK_STATUS_LABELS[status], tone: 'warn' }
    default:
      return { label: LINK_STATUS_LABELS.unchecked, tone: 'muted' }
  }
}

/**
 * 发布版本排序：发布时间降序（未知时间排后），同时间按版本标签升序。
 * 与后端保持一致，用于后台编辑后即时预览。
 */
export function sortReleases(releases?: ResourceRelease[] | null): ResourceRelease[] {
  if (!releases?.length) return []
  return [...releases].sort((a, b) => {
    const at = a.released_at ? Date.parse(a.released_at) : NaN
    const bt = b.released_at ? Date.parse(b.released_at) : NaN
    const aok = !Number.isNaN(at)
    const bok = !Number.isNaN(bt)
    if (aok && bok && at !== bt) return bt - at
    if (aok !== bok) return aok ? -1 : 1
    return a.version_label.localeCompare(b.version_label)
  })
}

/** 仅保留 http(s) 外链，避免渲染出 javascript: 等危险协议。 */
export function safeExternalUrl(url?: string | null): string | null {
  if (!url) return null
  return /^https?:\/\//i.test(url.trim()) ? url.trim() : null
}
