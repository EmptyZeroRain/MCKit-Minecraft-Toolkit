// ===== 反编译客户端（主线程）=====
// 职责：
//   1. 懒创建 / 复用反编译 Worker；
//   2. 按 (version, side) 取 jar：内存去重 + IndexedDB 持久缓存（jar 走 POST 下发，
//      浏览器不再对响应做 HTTP 缓存，故由前端自行持久化，避免刷新后重下几十 MB）；
//   3. 对外暴露「点号类名 → Java 源码」的单一入口。
import * as Comlink from 'comlink'
import type { McsrcDecompileResult, McsrcSide } from '@/types/mcsrc'
import { fetchJarBlob } from '@/api/mcsrc'
import { clearDeclarationCache } from './navigation'
import { clearJars, readJar, writeJar } from './jarStore'
import type { DecompileWorkerApi } from './decompile.worker'

export type { McsrcDecompileResult }

let worker: Comlink.Remote<DecompileWorkerApi> | null = null

/** 懒创建 Worker：模块顶层不触碰 Worker，便于在无 Worker 的环境（单测）中导入本模块。 */
function getWorker(): Comlink.Remote<DecompileWorkerApi> {
  if (!worker) {
    const raw = new Worker(new URL('./decompile.worker.ts', import.meta.url), {
      type: 'module',
      name: 'mcsrc-decompiler',
    })
    worker = Comlink.wrap<DecompileWorkerApi>(raw)
  }
  return worker
}

/** jar 存储键（IndexedDB）：<version>-<side> */
export function jarKey(version: string, side: McsrcSide): string {
  return `${version}-${side}`
}

/**
 * jar 身份：已知官方摘要时追加 `@<sha1>`。
 *
 * 同时用作内存缓存键、结果缓存键与 Worker 内 zip 的键：服务端重新准备后摘要变化，
 * 三者会一并失效，避免用旧 jar 去反编译新索引里的类。
 */
function jarIdentity(version: string, side: McsrcSide, sha1?: string): string {
  const base = jarKey(version, side)
  return sha1 ? `${base}@${sha1}` : base
}

// 同一 jar 只取一次；失败即从缓存移除，允许重试。
const jarCache = new Map<string, Promise<Blob>>()

// 反编译结果缓存：跳转时在同一批类之间来回不重复反编译（单类约 1s）。
const resultCache = new Map<string, Promise<McsrcDecompileResult>>()

/**
 * 取 jar 字节：先查 IndexedDB 持久缓存，未命中再回源下载。
 *
 * expectedSha1 来自版本状态（`/mcsrc/status`）：与缓存记录不一致说明服务端已重新
 * 准备过该版本，需重新下载；摘要未知时只能信任缓存。
 */
async function loadJar(version: string, side: McsrcSide, expectedSha1?: string): Promise<Blob> {
  const storeKey = jarKey(version, side)
  const cached = await readJar(storeKey)
  if (cached && (!expectedSha1 || !cached.sha1 || cached.sha1 === expectedSha1)) {
    return cached.blob
  }
  const { blob, sha1 } = await fetchJarBlob(version, side)
  await writeJar({ key: storeKey, sha1: sha1 || expectedSha1 || '', blob, savedAt: Date.now() })
  return blob
}

/** 取 jar（内存去重 + IndexedDB 持久缓存，命中则不重复下载） */
export function getJar(version: string, side: McsrcSide, expectedSha1?: string): Promise<Blob> {
  const key = jarIdentity(version, side, expectedSha1)
  let pending = jarCache.get(key)
  if (!pending) {
    pending = loadJar(version, side, expectedSha1)
    jarCache.set(key, pending)
    pending.catch(() => jarCache.delete(key))
  }
  return pending
}

/** 反编译单个类（点号全限定名）；expectedSha1 用于校验 jar 缓存是否过期。 */
export async function decompileClass(
  version: string,
  side: McsrcSide,
  className: string,
  expectedSha1?: string,
): Promise<McsrcDecompileResult> {
  const jarName = jarIdentity(version, side, expectedSha1)
  const key = `${jarName}|${className}`
  let pending = resultCache.get(key)
  if (!pending) {
    pending = (async () => {
      const jar = await getJar(version, side, expectedSha1)
      return getWorker().decompileClass({ className, jarName, jar })
    })()
    resultCache.set(key, pending)
    pending.catch(() => resultCache.delete(key))
  }
  return pending
}

/** 清空 jar 与反编译结果缓存（如版本被重新准备后调用） */
export function clearJarCache(): void {
  jarCache.clear()
  resultCache.clear()
  clearDeclarationCache()
  // IndexedDB 清理是异步的；即便失败，内存缓存也已清空。
  void clearJars()
}
