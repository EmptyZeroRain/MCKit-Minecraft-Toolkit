// @run-slicer/vf（以 npm 别名 vf-1.12.0 安装）的 WASM 运行时与 JS 回退包
// 均未提供类型声明，这里按上游 mcsrc 的做法补齐。
//
// 主入口 vf-1.12.0 自带 vf.d.ts（Options / Config / TokenCollector / decompile），
// 无需在此重复声明。

declare module 'vf-1.12.0/vf.wasm-runtime.js' {
  export function load(
    wasmPath: string,
    options?: { noAutoImports?: boolean },
  ): Promise<{ exports: typeof import('vf-1.12.0') }>
}

declare module 'vf-1.12.0/vf.runtime.js' {
  export const decompile: typeof import('vf-1.12.0').decompile
}
