// ===== Vineflower 运行时（Vineflower 反编译器的 WASM / JS 移植版）=====
// 反编译完全在浏览器内完成，**不依赖部署侧 JVM**：
//   优先加载 WASM（vf.wasm），浏览器不支持 WasmGC 等特性时回退纯 JS 运行时。
//
// 与上游 mcsrc 一致：显式用 Vite 的 `?url` 解析 wasm 资源再交给 load()，
// 而不是依赖包内 `new URL('./vf.wasm', import.meta.url)`——后者在依赖预打包
// （esbuild optimizeDeps）下不一定被 Vite 重写为产物 URL。
import wasmUrl from 'vf-1.12.0/vf.wasm?url'
import { load } from 'vf-1.12.0/vf.wasm-runtime.js'
import type { Config } from 'vf-1.12.0'

type Runtime = typeof import('vf-1.12.0')

let runtime: Runtime | null = null

/** 加载运行时（幂等）。优先 WASM，失败回退纯 JS。 */
export async function loadRuntime(): Promise<void> {
  if (runtime) return
  try {
    const { exports } = await load(wasmUrl, { noAutoImports: true })
    runtime = exports
    return
  } catch (err) {
    // 非合规浏览器（缺少 WasmGC 等）走纯 JS 回退，功能一致但更慢。
    console.warn('[mcsrc] Vineflower WASM 加载失败，回退到纯 JS 运行时', err)
  }
  runtime = (await import('vf-1.12.0/vf.runtime.js')) as unknown as Runtime
}

/** 反编译一个或多个类，返回「内部类名 → 源码」。调用前须先 loadRuntime()。 */
export async function decompile(
  names: string | string[],
  config?: Config,
): Promise<Record<string, string>> {
  if (!runtime) throw new Error('Vineflower 运行时尚未加载')
  return runtime.decompile(names, config)
}
