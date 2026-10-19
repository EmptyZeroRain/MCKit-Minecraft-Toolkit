<script setup lang="ts">
// 空状态：无结果时的友好提示与建议动作
defineProps<{
  title?: string
  description?: string
  /** 建议动作按钮 */
  actionLabel?: string
}>()

defineEmits<{
  action: []
}>()
</script>

<template>
  <div class="flex flex-col items-center justify-center rounded-card border border-dashed border-border-light bg-ink-900/50 px-6 py-14 text-center">
    <div class="mb-4 flex h-14 w-14 items-center justify-center rounded-full bg-ink-700 text-text-muted">
      <svg class="h-7 w-7" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
        <path stroke-linecap="round" stroke-linejoin="round" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
      </svg>
    </div>
    <h3 class="mb-1 font-semibold text-text-primary">{{ title ?? '暂无内容' }}</h3>
    <p v-if="description" class="max-w-md text-sm text-text-secondary">{{ description }}</p>
    <button
      v-if="actionLabel"
      type="button"
      class="mt-5 inline-flex items-center gap-2 rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500"
      @click="$emit('action')"
    >
      {{ actionLabel }}
    </button>
  </div>
</template>