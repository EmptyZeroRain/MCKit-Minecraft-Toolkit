// ===== Monaco 编辑器环境配置 =====
// 反编译源码只需 Java 基础语言（Monaco 内置 basic-languages，走主线程词法着色），
// 因此只注册 editor worker；不需要 json / ts / css / html 的独立 worker。
//
// 注意：monaco-editor 0.52+ 的 exports 映射为 `"./*": "./esm/vs/*.js"`，
// 故子路径必须写成 `monaco-editor/editor/editor.worker`（而非 `monaco-editor/esm/vs/...`）。
import * as monaco from 'monaco-editor'
import EditorWorker from 'monaco-editor/editor/editor.worker?worker'

;(globalThis as unknown as { MonacoEnvironment: { getWorker: () => Worker } }).MonacoEnvironment = {
  getWorker: () => new EditorWorker(),
}

export { monaco }
