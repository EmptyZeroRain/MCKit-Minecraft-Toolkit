<script setup lang="ts">
// Monaco 只读代码查看器。
// 本组件被 `defineAsyncComponent` 懒加载：只有打开源码面板时才载入 monaco（体积较大）。
//
// 除渲染外还承担「token 着色 + 跳转」：
//   1. 依据反编译 token 给类名 / 成员着色，可跳转的 token 显示手型光标与点状下划线；
//   2. 注册 Java 的定义提供器与编辑器打开器，使 Ctrl/⌘ + 点击、F12、右键「转到定义」生效；
//   3. 支持在打开的源码中定位并选中指定成员声明（跳转落点）。
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { monaco } from '@/mcsrc/monaco'
import { findDeclarationToken, findTokenAtOffset, offsetAtPosition, parseMemberFragment, tokenMemberFragment } from '@/mcsrc/tokens'
import { toDottedClassName, toInternalClassName } from '@/mcsrc/names'
import { useThemeStore } from '@/stores/theme'
import type { McsrcJumpTarget, McsrcRevealTarget, McsrcToken } from '@/types/mcsrc'

const props = defineProps<{
  /** 源码文本 */
  source: string
  /** Monaco 语言 id，默认 java */
  language?: string
  /** 反编译 token（着色 / 跳转） */
  tokens?: McsrcToken[]
  /** 打开后需要定位的成员声明 */
  reveal?: McsrcRevealTarget | null
}>()

const emit = defineEmits<{
  /** 请求跳转到某个类（可附带成员定位） */
  jump: [target: McsrcJumpTarget]
}>()

// 自定义 URI scheme：Monaco 的「转到定义」只认 URI，用它把目标带回本组件再抛给页面。
const GOTO_SCHEME = 'mcsrc-goto'

const theme = useThemeStore()
const host = ref<HTMLElement | null>(null)
let editor: monaco.editor.IStandaloneCodeEditor | null = null
let decorations: monaco.editor.IEditorDecorationsCollection | null = null
let disposables: monaco.IDisposable[] = []

/** 站点明暗主题 → Monaco 主题 */
function monacoTheme(): string {
  return theme.isDark ? 'vs-dark' : 'vs'
}

/** 是否可跳转：非声明处，且目标类在该 jar 内 */
function jumpable(token: McsrcToken): boolean {
  return !token.declaration && token.inJar
}

/** 应用 token 装饰（按类型着色 + 可跳转手型） */
function applyDecorations() {
  if (!editor) return
  const model = editor.getModel()
  if (!model) return
  const items = (props.tokens ?? []).map((token) => {
    const start = model.getPositionAt(token.start)
    const end = model.getPositionAt(token.start + token.length)
    return {
      range: new monaco.Range(start.lineNumber, start.column, end.lineNumber, end.column),
      options: {
        inlineClassName: `mcsrc-token-${token.type}${jumpable(token) ? ' mcsrc-token-jumpable' : ''}`,
      },
    }
  })
  decorations?.clear()
  decorations = editor.createDecorationsCollection(items)
}

/** 取某位置上的 token（行列 → 字符偏移 → token） */
function tokenAt(position: monaco.IPosition): McsrcToken | null {
  const model = editor?.getModel()
  if (!model) return null
  const offset = offsetAtPosition(model.getLinesContent(), position.lineNumber, position.column)
  return findTokenAtOffset(props.tokens, offset)
}

/** 在已渲染的源码中定位并选中成员声明 */
function revealMember(target: McsrcRevealTarget | null | undefined) {
  const model = editor?.getModel()
  if (!editor || !model || !target) return
  const token = findDeclarationToken(props.tokens, target)
  if (!token) return
  const start = model.getPositionAt(token.start)
  const end = model.getPositionAt(token.start + token.length)
  const range = new monaco.Range(start.lineNumber, start.column, end.lineNumber, end.column)
  editor.setSelection(range)
  editor.revealRangeInCenter(range)
}

/** 注册跳转相关语言能力（定义提供器 / 悬停提示 / 编辑器打开器） */
function registerNavigation() {
  disposables.push(
    monaco.languages.registerDefinitionProvider('java', {
      provideDefinition(_model, position) {
        const token = tokenAt(position)
        if (!token || !jumpable(token)) return null
        const fragment = tokenMemberFragment(token)
        const uri = monaco.Uri.parse(
          `${GOTO_SCHEME}://class/${toInternalClassName(token.className)}${fragment ? `#${fragment}` : ''}`,
        )
        return {
          uri,
          range: new monaco.Range(
            position.lineNumber,
            position.column,
            position.lineNumber,
            position.column + token.length,
          ),
        }
      },
    }),
    monaco.languages.registerHoverProvider('java', {
      provideHover(_model, position) {
        const token = tokenAt(position)
        if (!token || !jumpable(token)) return null
        const target = token.name ? `${token.className}.${token.name}` : token.className
        return {
          contents: [{ value: `**跳转到** \`${target}\`` }, { value: 'Ctrl/⌘ + 点击，或按 F12' }],
        }
      },
    }),
    monaco.editor.registerEditorOpener({
      openCodeEditor(_source, resource) {
        if (resource.scheme !== GOTO_SCHEME) return false
        const internal = resource.path.replace(/^\/+/, '')
        if (!internal) return false
        emit('jump', {
          className: toDottedClassName(internal),
          reveal: parseMemberFragment(resource.fragment),
        })
        return true
      },
    }),
  )
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
  applyDecorations()
  registerNavigation()
  revealMember(props.reveal)
})

// 切换类 / 重新反编译时替换内容，并重新应用 token 与跳转落点。
watch(
  () => props.source,
  (next) => {
    const model = editor?.getModel()
    if (model && model.getValue() !== next) model.setValue(next)
    applyDecorations()
    revealMember(props.reveal)
  },
)

// token 单独变化（例如先渲染后补齐）时刷新装饰。
watch(() => props.tokens, () => applyDecorations())

// 同一个类内跳转到另一个成员：源码不变，仅定位。
watch(
  () => props.reveal,
  (next) => revealMember(next),
)

// 跟随站点明暗主题。
watch(
  () => theme.isDark,
  () => monaco.editor.setTheme(monacoTheme()),
)

onBeforeUnmount(() => {
  for (const d of disposables) d.dispose()
  disposables = []
  decorations?.clear()
  decorations = null
  editor?.dispose()
  editor = null
})
</script>

<template>
  <div ref="host" class="h-full w-full" data-test="mcsrc-source-viewer" />
</template>
