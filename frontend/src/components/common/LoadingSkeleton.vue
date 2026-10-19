<script setup lang="ts">
// 骨架屏：列表加载占位
withDefaults(defineProps<{ count?: number }>(), { count: 6 })
</script>

<template>
  <!-- TODO §14：整个骨架屏作为**单一**状态区域播报（避免每个占位块重复播报），
       动画元素对读屏隐藏。 -->
  <div
    class="grid gap-4"
    :style="{ gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))' }"
    role="status"
    aria-live="polite"
    aria-busy="true"
    aria-label="内容加载中"
  >
    <div v-for="i in count" :key="i" class="card animate-pulse p-5" aria-hidden="true">
      <div class="mb-3 h-4 w-3/4 rounded bg-ink-600" />
      <div class="mb-2 h-3 w-full rounded bg-ink-700" />
      <div class="mb-4 h-3 w-2/3 rounded bg-ink-700" />
      <div class="flex gap-2">
        <div class="h-5 w-16 rounded-full bg-ink-600" />
        <div class="h-5 w-16 rounded-full bg-ink-600" />
      </div>
    </div>
  </div>
</template>
