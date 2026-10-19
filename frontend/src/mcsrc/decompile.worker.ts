// ===== 反编译 Web Worker =====
// 在 Worker 中运行 Vineflower（避免阻塞主线程），并把 jar 缓存在 Worker 内：
// 同一 jar 的多次反编译只解析一次 zip 中央目录。
//
// 通过 Comlink 暴露给主线程（见 ./decompiler.ts）。该模块**只被 Worker 引用**，
// 主线程仅以 `import type` 取类型，因此不会在页面上下文中执行。
import * as Comlink from 'comlink'
import { readBlob, type Entry } from '@katana-project/zip'
import type { McsrcDecompileRequest, McsrcDecompileResult } from '@/types/mcsrc'
import { decompile, loadRuntime } from './vineflower'
import { toInternalClassName } from './names'

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
      }
    }
    const sources = await decompile([internal], {
      // 按需提供任意类字节：Vineflower 反编译一个类时还需解析其引用的类型。
      source: async (name) => {
        const entry = jar.entries.get(name)
        return entry ? await entry.bytes() : null
      },
      resources: jar.classes,
      options: {},
    })
    return { className: req.className, source: sources[internal] ?? '', language: 'java' }
  },
}

/** Worker 对外接口（供主线程 Comlink.wrap 使用） */
export type DecompileWorkerApi = typeof api

Comlink.expose(api)
