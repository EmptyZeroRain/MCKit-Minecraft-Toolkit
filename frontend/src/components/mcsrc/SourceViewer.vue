<script setup lang="ts">
// Monaco 只读代码查看器。
// 本组件被 `defineAsyncComponent` 懒加载：只有打开源码面板时才载入 monaco（体积较大）。
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { monaco } from '@/mcsrc/monaco'
import { useThemeStore } from '@/stores/theme'

const props = defineProps<{
  /** 源码文本 */
  source: string
  /** Monaco 语言 id，默认 java */
  language?: string
}>()

const theme = useThemeStore()
const host = ref<HTMLElement | null>(null)
let editor: monaco.editor.IStandaloneCodeEditor | null = null

/** 站点明暗主题 → Monaco 主题 */
function monacoTheme(): string {
  return theme.isDark ? 'vs-dark' : 'vs'
}

onMounted(() => {
  if (!host.value) return
  editor = monaco.editor.create(host.value, {
    value: props.source,
    language: props.language ?? 'java',
    theme: monacoTheme(),
    readOnly: true,
    domReadOnly: true,
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
    automaticLayout: true,
    fontSize: 13,
    tabSize: 4,
    renderLineHighlight: 'none',
  })
})

// 切换类 / 重新反编译时替换内容。
watch(
  () => props.source,
  (next) => {
    const model = editor?.getModel()
    if (model && model.getValue() !== next) model.setValue(next)
  },
)

// 跟随站点明暗主题。
watch(
  () => theme.isDark,
  () => monaco.editor.setTheme(monacoTheme()),
)

onBeforeUnmount(() => {
  editor?.dispose()
  editor = null
})
</script>

<template>
  <div ref="host" class="h-full w-full" data-test="mcsrc-source-viewer" />
</template>
