// ===== 反编译客户端（主线程）=====
// 职责：
//   1. 懒创建 / 复用反编译 Worker；
//   2. 按 (version, side) 拉取并缓存 jar（同一版本只从后端下载一次，避免重复传输）；
//   3. 对外暴露「点号类名 → Java 源码」的单一入口。
import * as Comlink from 'comlink'
import type { McsrcDecompileResult, McsrcSide } from '@/types/mcsrc'
import { fetchJarBlob } from '@/api/mcsrc'
import { clearDeclarationCache } from './navigation'
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

/** jar 缓存键：<version>-<side> */
export function jarKey(version: string, side: McsrcSide): string {
  return `${version}-${side}`
}

// 同一 (version, side) 的 jar 只取一次；失败即从缓存移除，允许重试。
const jarCache = new Map<string, Promise<Blob>>()

// 反编译结果缓存：跳转时在同一批类之间来回不重复反编译（单类约 1s）。
const resultCache = new Map<string, Promise<McsrcDecompileResult>>()

/** 取 jar（命中缓存则不重复下载） */
export function getJar(version: string, side: McsrcSide): Promise<Blob> {
  const key = jarKey(version, side)
  let pending = jarCache.get(key)
  if (!pending) {
    pending = fetchJarBlob(version, side)
    jarCache.set(key, pending)
    pending.catch(() => jarCache.delete(key))
  }
  return pending
}

/** 反编译单个类（点号全限定名） */
export async function decompileClass(
  version: string,
  side: McsrcSide,
  className: string,
): Promise<McsrcDecompileResult> {
  const key = `${jarKey(version, side)}|${className}`
  let pending = resultCache.get(key)
  if (!pending) {
    pending = (async () => {
      const jar = await getJar(version, side)
      return getWorker().decompileClass({ className, jarName: jarKey(version, side), jar })
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
}
