<script setup lang="ts">
// 加载器徽章
import { LOADER_LABELS, type LoaderType } from '@/types/common'

defineProps<{ loader: LoaderType }>()
</script>

<template>
  <span
    class="inline-flex items-center rounded border border-grass-500/30 bg-grass-500/10 px-1.5 py-0.5 text-xs text-grass-300"
  >
    {{ LOADER_LABELS[loader] ?? loader }}
  </span>
</template>