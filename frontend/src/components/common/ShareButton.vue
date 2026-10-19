<script setup lang="ts">
// 分享按钮（TODO §14）：复制当前页面链接，成功后轻提示。
//
// 安全性：默认链接取自地址栏（Cookie / CSRF 从不进入 URL），并在复制前
// 用 `sanitizeShareUrl` 剥离疑似机密的查询参数与 hash 片段，
// 因此「分享链接不含会话 Cookie、CSRF Token 或其他秘密信息」是**构造性保证**。
import { ref } from 'vue'
import { copyToClipboard } from '@/utils/clipboard'
import { sanitizeShareUrl } from '@/utils/share'
import { useUiStore } from '@/stores/ui'

const props = defineProps<{
  /** 自定义分享 URL；不传时取当前地址栏 */
  url?: string
  label?: string
  /** 成功提示文案 */
  successMessage?: string
}>()

const ui = useUiStore()
const copied = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined

async function handleShare() {
  const raw = props.url ?? (typeof window !== 'undefined' ? window.location.href : '')
  const ok = await copyToClipboard(sanitizeShareUrl(raw))
  if (ok) {
    copied.value = true
    ui.success(props.successMessage ?? '链接已复制，可直接分享')
    timer && clearTimeout(timer)
    timer = setTimeout(() => (copied.value = false), 2000)
  } else {
    ui.error('复制失败，请手动从地址栏复制链接')
  }
}
</script>

<template>
  <button
    type="button"
    class="inline-flex items-center gap-1.5 rounded-md border border-border-light bg-ink-700 px-2.5 py-1.5 text-xs text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
    :class="{ '!border-grass-500 !text-grass-300': copied }"
    :aria-label="label ?? '复制链接'"
    @click="handleShare"
  >
    <svg
      class="h-3.5 w-3.5"
      fill="none"
      viewBox="0 0 24 24"
      stroke="currentColor"
      stroke-width="2"
      aria-hidden="true"
    >
      <path
        stroke-linecap="round"
        stroke-linejoin="round"
        d="M8.684 13.342A3 3 0 006.75 12.75a3 3 0 100 6 3 3 0 002.684-1.842m4.632-6.216A3 3 0 0011.25 12.75a3 3 0 100 6 3 3 0 002.684-1.842M15 12.75l-6-1.5"
      />
      <path
        stroke-linecap="round"
        stroke-linejoin="round"
        d="M13.5 6.75h6v6m0-6L9 17.25"
      />
    </svg>
    <span>{{ copied ? '已复制' : (label ?? '复制链接') }}</span>
  </button>
</template>
