<script setup lang="ts">
// 游戏版本徽章
defineProps<{ version: string }>()
</script>

<template>
  <span
    class="inline-flex items-center rounded border border-border-light bg-ink-700 px-1.5 py-0.5 text-xs text-text-secondary"
  >
    {{ version }}
  </span>
</template>