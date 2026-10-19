<script setup lang="ts">
// 错误状态：请求失败时的统一反馈与重试
defineProps<{
  title?: string
  message?: string
  /** 是否展示重试按钮 */
  retryable?: boolean
}>()

defineEmits<{
  retry: []
}>()
</script>

<template>
  <!-- TODO §14：错误状态统一以 role="alert" 播报，读屏用户无需主动寻找错误文案 -->
  <div
    role="alert"
    class="flex flex-col items-center justify-center rounded-card border border-danger-mc/30 bg-ink-900/50 px-6 py-14 text-center"
  >
    <div class="mb-4 flex h-14 w-14 items-center justify-center rounded-full bg-danger-mc/10 text-danger-mc">
      <svg class="h-7 w-7" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
        <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
      </svg>
    </div>
    <h3 class="mb-1 font-semibold text-text-primary">{{ title ?? '加载失败' }}</h3>
    <p class="max-w-md text-sm text-text-secondary">{{ message ?? '数据暂时无法加载，请稍后重试。' }}</p>
    <button
      v-if="retryable"
      type="button"
      class="mt-5 inline-flex items-center gap-2 rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm font-medium text-text-primary transition hover:bg-ink-600"
      @click="$emit('retry')"
    >
      重试
    </button>
  </div>
</template>