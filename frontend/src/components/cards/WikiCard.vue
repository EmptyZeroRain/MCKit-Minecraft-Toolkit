<script setup lang="ts">
// 百科条目卡片
import { RouterLink } from 'vue-router'
import type { WikiEntry } from '@/types/wiki'
import { timeAgo } from '@/utils/format'

defineProps<{ entry: WikiEntry }>()

const contentTypeLabel: Record<string, string> = {
  guide: '指南',
  concept: '概念',
  tutorial: '教程',
  reference: '参考',
  other: '其他',
}
</script>

<template>
  <article class="card card-hover flex flex-col p-5">
    <div class="mb-1 flex items-center gap-2 text-xs text-text-muted">
      <span
        class="rounded border border-border-light bg-ink-700 px-1.5 py-0.5"
      >
        {{ contentTypeLabel[entry.content_type] ?? entry.content_type }}
      </span>
      <span>更新于 {{ timeAgo(entry.updated_at) }}</span>
    </div>
    <RouterLink
      :to="`/wiki/${entry.slug}`"
      class="mb-2 font-semibold text-text-primary hover:text-grass-300"
    >
      {{ entry.title }}
    </RouterLink>
    <p class="line-clamp-2 flex-1 text-sm leading-relaxed text-text-secondary">
      {{ entry.summary }}
    </p>
    <div class="mt-3 flex flex-wrap gap-1.5">
      <span
        v-for="tag in entry.tags.slice(0, 4)"
        :key="tag"
        class="rounded bg-ink-700 px-1.5 py-0.5 text-xs text-text-muted"
      >
        #{{ tag }}
      </span>
    </div>
  </article>
</template>