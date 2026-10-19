// ===== 资源后台表单 ↔ 接口载荷转换（TODO §6）=====
//
// 目的：把「逗号分隔文本 / 日期字符串」等表单友好表示与接口载荷分离，
// 使转换与校验可独立单测，Vue 组件只负责渲染。
import { LOADER_TYPES, type LoaderType } from '@/types/common'
import { ALL_VERSIONS, ALL_VERSIONS_LABEL } from '@/types/version'
import type {
  ResourceDependency,
  ResourceDependencyType,
  ResourceRelease,
  ResourceReleasePayload,
} from '@/types/resource'
import { normalizeDependencyType } from '@/utils/resource'

/** 依赖项表单模型 */
export interface DependencyForm {
  name: string
  type: ResourceDependencyType
  resource_url: string
  required: boolean
  note: string
}

/** 发布版本表单模型（版本 / 加载器以逗号分隔文本编辑） */
export interface ReleaseForm {
  version_label: string
  game_versions_text: string
  loader_types_text: string
  source_url: string
  /** yyyy-MM-dd；留空表示发布时间未知 */
  released_at: string
  notes: string
  is_latest: boolean
}

/** 逗号（中英文）分隔文本 → 去空白后的字符串数组 */
export function splitList(text: string | null | undefined): string[] {
  if (!text) return []
  return text
    .split(/[,，]/)
    .map((s) => s.trim())
    .filter(Boolean)
}

/** 字符串数组 → 逗号分隔文本 */
export function joinListText(items: string[] | null | undefined): string {
  return (items ?? []).join(', ')
}

/** 仅保留 yyyy-MM-dd 部分（接口可能返回 RFC3339） */
export function toDateInputValue(iso: string | null | undefined): string {
  if (!iso) return ''
  const m = /^(\d{4}-\d{2}-\d{2})/.exec(iso.trim())
  return m ? m[1] : ''
}

/** 接口依赖项 → 表单模型 */
export function dependencyToForm(dep: ResourceDependency): DependencyForm {
  return {
    name: dep.name ?? '',
    type: normalizeDependencyType(dep.type),
    resource_url: dep.resource_url ?? '',
    required: dep.required === true,
    note: dep.note ?? '',
  }
}

/** 表单模型 → 接口依赖项（冲突项不携带 required，避免语义矛盾） */
export function formToDependency(form: DependencyForm): ResourceDependency {
  const type = normalizeDependencyType(form.type)
  const out: ResourceDependency = {
    name: form.name.trim(),
    type,
    required: type === 'conflict' ? false : form.required,
  }
  if (form.resource_url.trim()) out.resource_url = form.resource_url.trim()
  if (form.note.trim()) out.note = form.note.trim()
  return out
}

/** 接口发布版本 → 表单模型 */
export function releaseToForm(rel: ResourceRelease): ReleaseForm {
  return {
    version_label: rel.version_label ?? '',
    game_versions_text: joinListText(rel.game_versions),
    loader_types_text: joinListText(rel.loader_types),
    source_url: rel.source_url ?? '',
    released_at: toDateInputValue(rel.released_at),
    notes: rel.notes ?? '',
    is_latest: rel.is_latest === true,
  }
}

/** 表单模型 → 接口载荷 */
export function formToReleasePayload(form: ReleaseForm): ResourceReleasePayload {
  return {
    version_label: form.version_label.trim(),
    game_versions: splitList(form.game_versions_text),
    loader_types: splitList(form.loader_types_text) as LoaderType[],
    source_url: form.source_url.trim(),
    released_at: form.released_at.trim() || null,
    notes: form.notes.trim(),
    is_latest: form.is_latest,
  }
}

/**
 * 校验版本声明文本（逗号分隔）。
 *
 * 只做**语义**校验（通配必须独占），格式校验交由后端 `versioning` 统一裁决，
 * 避免前端正则与后端规则漂移导致误拦截。
 */
export function validateVersionText(text: string): string | null {
  const items = splitList(text)
  if (items.length === 0) return null
  const hasAll = items.some((v) => v === ALL_VERSIONS || v === '*')
  if (hasAll && items.length > 1) {
    return `声明「${ALL_VERSIONS_LABEL}」时不能同时填写其他版本`
  }
  return null
}

/**
 * 校验发布版本表单，返回首条错误信息（无错误返回 null）。
 * 与后端规则保持一致：版本标签必填且唯一、来源必填且为 http(s)、加载器在枚举内、
 * 至多一条「最新」。
 */
export function validateReleaseForms(forms: ReleaseForm[]): string | null {
  const seen = new Set<string>()
  let latest = 0
  for (let i = 0; i < forms.length; i++) {
    const f = forms[i]
    const label = f.version_label.trim()
    const at = `第 ${i + 1} 个发布版本`
    if (!label) return `${at}：版本标签必填`
    if (seen.has(label)) return `${at}：版本标签重复「${label}」`
    seen.add(label)
    if (!f.source_url.trim()) return `${at}：发布页链接必填`
    if (!/^https?:\/\/\S+$/i.test(f.source_url.trim())) return `${at}：发布页链接必须以 http(s):// 开头`
    const versionError = validateVersionText(f.game_versions_text)
    if (versionError) return `${at}：${versionError}`
    for (const l of splitList(f.loader_types_text)) {
      if (!(LOADER_TYPES as readonly string[]).includes(l)) {
        return `${at}：不支持的加载器「${l}」`
      }
    }
    if (f.is_latest) latest++
  }
  if (latest > 1) return '只能有一个发布版本标记为「最新」'
  return null
}

/** 校验依赖表单，返回首条错误信息（无错误返回 null）。 */
export function validateDependencyForms(forms: DependencyForm[]): string | null {
  for (let i = 0; i < forms.length; i++) {
    const f = forms[i]
    const at = `第 ${i + 1} 条依赖`
    if (!f.name.trim()) return `${at}：名称必填`
    if (f.resource_url.trim() && !/^https?:\/\/\S+$/i.test(f.resource_url.trim())) {
      return `${at}：链接必须以 http(s):// 开头`
    }
  }
  return null
}
