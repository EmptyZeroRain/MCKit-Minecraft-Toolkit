// ===== 反编译 Web Worker =====
// 在 Worker 中运行 Vineflower（避免阻塞主线程），并把 jar 缓存在 Worker 内：
// 同一 jar 的多次反编译只解析一次 zip 中央目录。
//
// 通过 Comlink 暴露给主线程（见 ./decompiler.ts）。该模块**只被 Worker 引用**，
// 主线程仅以 `import type` 取类型，因此不会在页面上下文中执行。
import * as Comlink from 'comlink'
import { readBlob, type Entry } from '@katana-project/zip'
import type { McsrcDecompileRequest, McsrcDecompileResult, McsrcToken } from '@/types/mcsrc'
import { decompile, loadRuntime } from './vineflower'
import { classLookupCandidates, toDottedClassName, toInternalClassName } from './names'

interface OpenedJar {
  /** 内部类名 → zip 条目 */
  entries: Map<string, Entry>
  /** 全部类名（内部名），供 Vineflower 解析引用关系 */
  classes: string[]
}

// jar 只在 Worker 内解析一次；jar 键变化时替换。
let openedKey = ''
let opened: OpenedJar | null = null

async function openJar(key: string, blob: Blob): Promise<OpenedJar> {
  if (openedKey === key && opened) return opened
  const zip = await readBlob(blob, { naive: true })
  const entries = new Map<string, Entry>()
  const classes: string[] = []
  for (const entry of zip.entries) {
    if (!entry.name.endsWith('.class')) continue
    const name = entry.name.slice(0, -'.class'.length)
    entries.set(name, entry)
    classes.push(name)
  }
  openedKey = key
  opened = { entries, classes }
  return opened
}

/**
 * 把源码里出现的类名对齐到 jar 条目 / 后端索引的约定。
 * 反编译源码中内部类写作 `Outer.Inner`，jar 条目与后端索引用 `Outer$Inner`，
 * 因此按候选序列探测 jar 条目集合；未命中说明该类不在 jar 内（JDK / 第三方库），标记为不可跳转。
 */
function resolveClassName(
  entries: Map<string, Entry>,
  rawName: string,
): { className: string; inJar: boolean } {
  for (const candidate of classLookupCandidates(rawName)) {
    if (entries.has(candidate)) return { className: toDottedClassName(candidate), inJar: true }
  }
  return { className: toDottedClassName(toInternalClassName(rawName)), inJar: false }
}

// 反编译源码中的 import 语句：类型以简单名出现，import 行本身也应可点击跳转。
const IMPORT_RE = /^\s*import\s+(?!static\b)([^\s;]+)\s*;/gm

/**
 * 为 import 语句补 class token（上游 mcsrc 同样做法）。
 * 与上游的差异：token 覆盖**整个**导入名（含包路径）而不是仅简单名，
 * 这样点击 `net.minecraft.world.World` 的任意位置都能跳转。
 */
function importTokens(source: string, entries: Map<string, Entry>): McsrcToken[] {
  const out: McsrcToken[] = []
  IMPORT_RE.lastIndex = 0
  let match: RegExpExecArray | null
  while ((match = IMPORT_RE.exec(source)) !== null) {
    const imported = match[1]
    if (imported.endsWith('*')) continue
    const offset = match[0].lastIndexOf(imported)
    if (offset < 0) continue
    const resolved = resolveClassName(entries, imported)
    out.push({
      type: 'class',
      start: match.index + offset,
      length: imported.length,
      declaration: false,
      className: resolved.className,
      inJar: resolved.inJar,
    })
  }
  return out
}

/** 收集反编译引擎产出的 token（Vineflower 回调给出的是 JVM 内部名，需对齐到 jar 约定） */
function createTokenCollector(entries: Map<string, Entry>, sink: McsrcToken[]) {
  let current: McsrcToken[] = []
  return {
    start() {
      current = []
    },
    visitClass(start: number, length: number, declaration: boolean, name: string) {
      current.push({
        type: 'class',
        start,
        length,
        declaration,
        ...resolveClassName(entries, name),
      })
    },
    visitField(
      start: number,
      length: number,
      declaration: boolean,
      className: string,
      name: string,
      descriptor: string,
    ) {
      current.push({
        type: 'field',
        start,
        length,
        declaration,
        name,
        descriptor,
        ...resolveClassName(entries, className),
      })
    },
    visitMethod(
      start: number,
      length: number,
      declaration: boolean,
      className: string,
      name: string,
      descriptor: string,
    ) {
      current.push({
        type: 'method',
        start,
        length,
        declaration,
        name,
        descriptor,
        ...resolveClassName(entries, className),
      })
    },
    visitParameter(
      start: number,
      length: number,
      declaration: boolean,
      className: string,
      _methodName: string,
      _methodDescriptor: string,
      _index: number,
      _name: string,
    ) {
      current.push({
        type: 'parameter',
        start,
        length,
        declaration,
        ...resolveClassName(entries, className),
      })
    },
    visitLocal(
      start: number,
      length: number,
      declaration: boolean,
      className: string,
      _methodName: string,
      _methodDescriptor: string,
      _index: number,
      _name: string,
    ) {
      current.push({
        type: 'local',
        start,
        length,
        declaration,
        ...resolveClassName(entries, className),
      })
    },
    end() {
      sink.push(...current)
      current = []
    },
  }
}

const api = {
  /** 反编译单个类。类不存在时返回提示文本（不抛错，避免打断面板渲染）。 */
  async decompileClass(req: McsrcDecompileRequest): Promise<McsrcDecompileResult> {
    await loadRuntime()
    const jar = await openJar(req.jarName, req.jar)
    const internal = toInternalClassName(req.className)
    if (!jar.entries.has(internal)) {
      return {
        className: req.className,
        source: `// 该类不存在于该版本的 jar 中：${req.className}`,
        language: 'java',
        tokens: [],
      }
    }
    const tokens: McsrcToken[] = []
    const sources = await decompile([internal], {
      // 按需提供任意类字节：Vineflower 反编译一个类时还需解析其引用的类型。
      source: async (name) => {
        const entry = jar.entries.get(name)
        return entry ? await entry.bytes() : null
      },
      resources: jar.classes,
      options: {},
      tokenCollector: createTokenCollector(jar.entries, tokens),
    })
    const source = sources[internal] ?? ''
    tokens.push(...importTokens(source, jar.entries))
    // 按起始偏移排序：命中判定依赖有序性。
    tokens.sort((a, b) => a.start - b.start)
    return { className: req.className, source, language: 'java', tokens }
  },
}

/** Worker 对外接口（供主线程 Comlink.wrap 使用） */
export type DecompileWorkerApi = typeof api

Comlink.expose(api)
