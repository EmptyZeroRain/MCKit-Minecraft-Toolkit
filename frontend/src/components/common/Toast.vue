<script setup lang="ts">
// 全局 Toast 出口：读取 ui store 渲染消息，支持键盘关闭
import { useUiStore, type ToastType } from '@/stores/ui'

const ui = useUiStore()

const icons: Record<ToastType, string> = {
  success: '✓',
  error: '✕',
  info: 'ℹ',
  warning: '!',
}

const colorClass: Record<ToastType, string> = {
  success: 'border-grass-500/50 bg-ink-800 text-grass-300',
  error: 'border-danger-mc/50 bg-ink-800 text-danger-mc',
  info: 'border-border-light bg-ink-800 text-text-primary',
  warning: 'border-amber-mc/50 bg-ink-800 text-amber-mc',
}
</script>

<template>
  <div
    class="pointer-events-none fixed bottom-5 left-1/2 z-[100] flex w-[min(92vw,26rem)] -translate-x-1/2 flex-col gap-2"
    aria-live="polite"
    role="status"
  >
    <TransitionGroup name="toast">
      <div
        v-for="toast in ui.toasts"
        :key="toast.id"
        class="pointer-events-auto flex items-center gap-3 rounded-lg border px-4 py-3 shadow-xl shadow-black/40"
        :class="colorClass[toast.type]"
      >
        <span class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full border text-xs font-bold">
          {{ icons[toast.type] }}
        </span>
        <span class="flex-1 text-sm">{{ toast.message }}</span>
        <button
          class="text-xs opacity-70 hover:opacity-100"
          aria-label="关闭提示"
          @click="ui.remove(toast.id)"
        >
          ✕
        </button>
      </div>
    </TransitionGroup>
  </div>
</template>

<style scoped>
.toast-enter-active,
.toast-leave-active {
  transition: all 0.25s ease;
}
.toast-enter-from {
  opacity: 0;
  transform: translateY(12px);
}
.toast-leave-to {
  opacity: 0;
  transform: translateY(8px);
}
</style>