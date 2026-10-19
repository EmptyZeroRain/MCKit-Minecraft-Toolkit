<script setup lang="ts">
// 管理列表外壳：统一标题、加载、错误处理
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import ErrorState from '@/components/common/ErrorState.vue'

defineProps<{
  title: string
  loading?: boolean
  error?: string
}>()

defineEmits<{ retry: [] }>()
</script>

<template>
  <section aria-labelledby="list-title">
    <div class="mb-4 flex items-center justify-between gap-3">
      <h2 id="list-title" class="text-lg font-semibold text-text-primary">{{ title }}</h2>
      <slot name="toolbar" />
    </div>

    <LoadingSkeleton v-if="loading" :count="5" class="card p-2" />
    <ErrorState v-else-if="error" :message="error" retryable @retry="$emit('retry')" />
    <slot v-else />
  </section>
</template>