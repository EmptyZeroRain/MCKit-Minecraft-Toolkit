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

watch(
  () => props.open,
  (val) => {
    if (val) {
      // 聚焦确认按钮，保证键盘可用
      requestAnimationFrame(() => {
        dialogEl.value
          ?.querySelector<HTMLButtonElement>('button[data-confirm]')
          ?.focus()
      })
    }
  },
)
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
    >
      <div
        ref="dialogEl"
        class="card w-full max-w-md p-6 shadow-2xl"
        @keydown.esc="emit('cancel')"
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