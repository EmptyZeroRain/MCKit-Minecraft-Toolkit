<script setup lang="ts">
// 分页组件
const props = defineProps<{
  page: number
  pageSize: number
  total: number
  totalPages: number
}>()

const emit = defineEmits<{
  change: [page: number]
}>()

function go(p: number) {
  if (p < 1 || p > props.totalPages || p === props.page) return
  emit('change', p)
}

const visiblePages = () => {
  const total = Math.max(props.totalPages, 1)
  const current = Math.min(Math.max(props.page, 1), total)
  const range: number[] = []
  const start = Math.max(1, current - 2)
  const end = Math.min(total, current + 2)
  for (let i = start; i <= end; i++) range.push(i)
  return { range, current, total }
}
</script>

<template>
  <nav v-if="totalPages > 1" class="flex flex-wrap items-center justify-center gap-1" aria-label="分页">
    <button
      type="button"
      class="inline-flex h-9 min-w-9 items-center justify-center rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-secondary transition hover:bg-ink-700 disabled:cursor-not-allowed disabled:opacity-40"
      :disabled="page <= 1"
      aria-label="上一页"
      @click="go(page - 1)"
    >
      ‹
    </button>

    <template v-for="p in visiblePages().range" :key="p">
      <button
        type="button"
        class="inline-flex h-9 min-w-9 items-center justify-center rounded-md px-3 text-sm transition"
        :class="
          p === visiblePages().current
            ? 'bg-grass-600 font-semibold text-white'
            : 'border border-border-dark bg-ink-800 text-text-secondary hover:bg-ink-700'
        "
        :aria-current="p === visiblePages().current ? 'page' : undefined"
        @click="go(p)"
      >
        {{ p }}
      </button>
    </template>

    <button
      type="button"
      class="inline-flex h-9 min-w-9 items-center justify-center rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-secondary transition hover:bg-ink-700 disabled:cursor-not-allowed disabled:opacity-40"
      :disabled="page >= totalPages"
      aria-label="下一页"
      @click="go(page + 1)"
    >
      ›
    </button>

    <span class="ml-3 text-xs text-text-muted">
      共 {{ total }} 条 · 第 {{ visiblePages().current }}/{{ visiblePages().total }} 页
    </span>
  </nav>
</template>