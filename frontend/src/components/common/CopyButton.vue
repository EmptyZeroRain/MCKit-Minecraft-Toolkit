<script setup lang="ts">
// 复制按钮：Clipboard API + 降级；成功后轻提示
import { ref } from 'vue'
import { copyToClipboard } from '@/utils/clipboard'
import { useUiStore } from '@/stores/ui'
import { reportToolUse } from '@/utils/tool-stats'

const props = defineProps<{
  text: string
  label?: string
  /** 成功提示文案 */
  successMessage?: string
  /**
   * 所属内置工具 slug（TODO §13）。
   * 传值后，复制成功即视为「使用」该工具——这是计算/生成类工具最明确的可观测使用信号。
   */
  tool?: string
}>()

const ui = useUiStore()
const copied = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined

async function handleCopy() {
  const ok = await copyToClipboard(props.text)
  if (ok) {
    copied.value = true
    ui.success(props.successMessage ?? '已复制到剪贴板')
    reportToolUse(props.tool)
    timer && clearTimeout(timer)
    timer = setTimeout(() => (copied.value = false), 2000)
  } else {
    // 复制失败：允许手动选中。提示并聚焦文本。
    ui.error('复制失败，请手动选择内容复制')
  }
}
</script>

<template>
  <button
    type="button"
    class="inline-flex items-center gap-1.5 rounded-md border border-border-light bg-ink-700 px-2.5 py-1.5 text-xs text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
    :class="{ '!border-grass-500 !text-grass-300': copied }"
    :aria-label="copied ? '已复制' : '复制'"
    @click="handleCopy"
  >
    <svg
      v-if="!copied"
      class="h-3.5 w-3.5"
      fill="none"
      viewBox="0 0 24 24"
      stroke="currentColor"
      stroke-width="2"
    >
      <path
        stroke-linecap="round"
        stroke-linejoin="round"
        d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
      />
    </svg>
    <svg
      v-else
      class="h-3.5 w-3.5"
      fill="none"
      viewBox="0 0 24 24"
      stroke="currentColor"
      stroke-width="2"
    >
      <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
    </svg>
    <span>{{ copied ? '已复制' : (label ?? '复制') }}</span>
  </button>
</template>