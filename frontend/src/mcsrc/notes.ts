// ===== 备注（Javadoc）本地存储 =====
//
// 上游 mcsrc 的 Javadoc 存储实为占位实现（readJavadoc 返回空、writeJavadoc 仅打日志），
// 本项目将其正式落地为**浏览器本地存储**：匿名访客即可使用，不新增数据库表、不要求登录、
// 不上传任何内容（备注只存在使用者本机）。
//
// 存储结构（单键，便于整体清理）：
//
//	{
//	  "<version>|<side>|<className>": {
//	    "class": "类备注",
//	    "methods": { "<name><descriptor>": "方法备注" },
//	    "fields":  { "<name><descriptor>": "字段备注" }
//	  }
//	}
//
// 环境不支持 localStorage（隐私模式、SSR）时全部降级为空操作：读取返回空串，
// 写入静默失败，不影响功能。
import { ref } from 'vue'
import type { McsrcNoteTarget } from '@/types/mcsrc'

const STORAGE_KEY = 'mckit-mcsrc-notes'

/** 备注写入版本号：写入后自增，供界面据此重新读取备注（触发重渲染）。 */
export const notesRevision = ref(0)

/** 单个类的备注集合（与上游 JavadocData.classes[className] 同构）。 */
interface ClassNotes {
  class: string
  methods: Record<string, string>
  fields: Record<string, string>
}

type NotesMap = Record<string, ClassNotes>

/** 读取整个备注表；不可用或损坏时返回空表。 */
function readAll(): NotesMap {
  try {
    if (typeof localStorage === 'undefined') return {}
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return {}
    const parsed: unknown = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object') return {}
    return parsed as NotesMap
  } catch {
    // 存储损坏 / 不可用：视为无备注，不抛错打断界面。
    return {}
  }
}

/** 写回整个备注表（失败静默忽略）。 */
function writeAll(map: NotesMap): void {
  try {
    if (typeof localStorage === 'undefined') return
    localStorage.setItem(STORAGE_KEY, JSON.stringify(map))
  } catch {
    // 配额满 / 隐私模式：静默失败，仅失去持久化收益。
  }
}

/** 类级存储键。 */
function classKey(target: McsrcNoteTarget): string {
  return `${target.version}|${target.side}|${target.className}`
}

/** 成员级存储键（与上游一致：名字 + 描述符）。 */
function memberKey(kind: 'method' | 'field', name: string, descriptor: string): string {
  return `${kind}:${name}${descriptor}`
}

/** 取某个类备注条目（不存在时返回空条目）。 */
function classEntry(map: NotesMap, target: McsrcNoteTarget): ClassNotes {
  return map[classKey(target)] ?? { class: '', methods: {}, fields: {} }
}

/** 读取类备注。 */
export function readClassNote(target: McsrcNoteTarget): string {
  return classEntry(readAll(), target).class
}

/** 读取成员备注（target 未指定成员时返回空串）。 */
export function readMemberNote(target: McsrcNoteTarget): string {
  if (!target.member) return ''
  const entry = classEntry(readAll(), target)
  const bucket = target.member.kind === 'method' ? entry.methods : entry.fields
  return bucket[memberKey(target.member.kind, target.member.name, target.member.descriptor)] ?? ''
}

/**
 * 写入类或成员备注。文本为空白时删除该条备注（避免留下空条目）。
 * 类内已无任何备注时整体移除该类条目。
 */
export function writeNote(target: McsrcNoteTarget, text: string): void {
  const map = readAll()
  const key = classKey(target)
  const entry = map[key] ?? { class: '', methods: {}, fields: {} }
  const value = text.trim()

  if (target.member) {
    const bucket = target.member.kind === 'method' ? entry.methods : entry.fields
    const mKey = memberKey(target.member.kind, target.member.name, target.member.descriptor)
    if (value) bucket[mKey] = text
    else delete bucket[mKey]
  } else if (value) {
    entry.class = text
  } else {
    entry.class = ''
  }

  const empty =
    !entry.class &&
    Object.keys(entry.methods).length === 0 &&
    Object.keys(entry.fields).length === 0
  if (empty) delete map[key]
  else map[key] = entry

  writeAll(map)
  notesRevision.value++
}

/** 某个类是否已有任何备注（用于展示标记）。 */
export function hasAnyNote(target: McsrcNoteTarget): boolean {
  const entry = classEntry(readAll(), target)
  return !!entry.class || Object.keys(entry.methods).length > 0 || Object.keys(entry.fields).length > 0
}

/** 清空全部备注（破坏性操作，调用方需二次确认）。 */
export function clearAllNotes(): void {
  try {
    if (typeof localStorage !== 'undefined') localStorage.removeItem(STORAGE_KEY)
  } catch {
    // 忽略：不可用时本就无备注。
  }
  notesRevision.value++
}
