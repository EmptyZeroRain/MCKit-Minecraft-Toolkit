<script setup lang="ts">
// 确认对话框：删除/下架前的二次确认（需求 9.2）
import { ref, watch } from 'vue'

const props = defineProps<{
  open: boolean
  title: string
  message: string
  confirmText?: string
  dangerous?: boolean
  /** 确认按钮禁用（用于进行中状态） */
  confirmDisabled?: boolean
}>()

const emit = defineEmits<{
  confirm: []
  cancel: []
}>()

const dialogEl = ref<HTMLDivElement | null>(null)
/** 打开前的焦点元素：关闭后要还原，否则键盘用户会「丢位置」到页面顶部。 */
let previousFocus: HTMLElement | null = null

watch(
  () => props.open,
  (val) => {
    if (val) {
      previousFocus = document.activeElement as HTMLElement | null
      // 聚焦确认按钮，保证键盘可用
      requestAnimationFrame(() => {
        dialogEl.value
          ?.querySelector<HTMLButtonElement>('button[data-confirm]')
          ?.focus()
      })
      return
    }
    previousFocus?.focus?.()
    previousFocus = null
  },
)

// focusables 返回对话框内可聚焦元素（用于 Tab 循环）。
function focusables(): HTMLElement[] {
  const root = dialogEl.value
  if (!root) return []
  return Array.from(
    root.querySelectorAll<HTMLElement>(
      'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
    ),
  )
}

// onKeydown 把焦点限制在对话框内（WAI-ARIA Dialog 模式要求）：
// 否则 Tab 会跑到背景页面，读屏软件仍能读到被遮挡的内容。
function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.stopPropagation()
    emit('cancel')
    return
  }
  if (e.key !== 'Tab') return
  const list = focusables()
  if (list.length === 0) return
  const first = list[0]
  const last = list[list.length - 1]
  const active = document.activeElement as HTMLElement | null
  if (e.shiftKey && (active === first || !list.includes(active as HTMLElement))) {
    e.preventDefault()
    last.focus()
  } else if (!e.shiftKey && (active === last || !list.includes(active as HTMLElement))) {
    e.preventDefault()
    first.focus()
  }
}
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="fixed inset-0 z-[90] flex items-center justify-center bg-black/60 p-4"
      role="dialog"
      aria-modal="true"
      :aria-label="title"
      @click.self="emit('cancel')"
      @keydown="onKeydown"
    >
      <div
        ref="dialogEl"
        class="card w-full max-w-md p-6 shadow-2xl"
      >
        <h2 class="mb-2 flex items-center gap-2 text-lg font-semibold text-text-primary">
          <svg
            v-if="dangerous"
            class="h-5 w-5 text-danger-mc"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            stroke-width="2"
          >
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
          </svg>
          {{ title }}
        </h2>
        <p class="mb-6 whitespace-pre-line text-sm leading-relaxed text-text-secondary">
          {{ message }}
        </p>
        <div class="flex justify-end gap-3">
          <button
            type="button"
            class="rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-primary transition hover:bg-ink-600"
            @click="emit('cancel')"
          >
            取消
          </button>
          <button
            type="button"
            data-confirm
            class="rounded-md px-4 py-2 text-sm font-medium text-white transition disabled:cursor-not-allowed disabled:opacity-50"
            :class="
              dangerous ? 'bg-danger-mc hover:bg-red-500' : 'bg-grass-600 hover:bg-grass-500'
            "
            :disabled="confirmDisabled"
            @click="emit('confirm')"
          >
            {{ confirmText ?? '确认' }}
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>