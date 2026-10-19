<script setup lang="ts">
// 搜索结果列表（需求 §4.3 / §5.1）：统一「类型徽标 + 高亮标题 + 高亮摘要」的渲染，
// 供全局搜索下拉（compact）与搜索结果页（card）复用，避免两处各写一套
// 关键词切分与 <mark> 渲染（不使用 v-html，防 XSS）。
import { RouterLink } from 'vue-router'
import type { SearchResultItem } from '@/types/search'
import { highlightSegments } from '@/utils/highlight'

const props = withDefaults(
  defineProps<{
    items: SearchResultItem[]
    /** 高亮关键词；为空时不高亮 */
    keyword?: string
    /** compact：下拉面板内的紧凑条目；card：结果页卡片 */
    variant?: 'compact' | 'card'
  }>(),
  { keyword: '', variant: 'card' },
)

const emit = defineEmits<{ select: [item: SearchResultItem] }>()

/** 搜索结果类型的中文名（未知类型回退原值）。 */
const TYPE_LABELS: Record<string, string> = {
  tool: '工具',
  resource: '资源',
  wiki: '百科',
  item: '物品',
  devdoc: '开发文档',
}

function typeLabel(type: string): string {
  return TYPE_LABELS[type] ?? type
}

/** 标题 / 摘要按关键词切分，供模板用 <mark> 渲染。 */
const titleSegments = (item: SearchResultItem) => highlightSegments(item.title, props.keyword)
const summarySegments = (item: SearchResultItem) =>
  highlightSegments(item.summary ?? '', props.keyword)

/** compact 变体自身不导航，交由父组件决定跳转（下拉需要先关闭面板）。 */
function onSelect(item: SearchResultItem) {
  if (props.variant === 'compact') emit('select', item)
}
</script>

<template>
  <ul :class="variant === 'card' ? 'space-y-3' : 'py-1'">
    <li
      v-for="item in items"
      :key="item.type + item.id"
      :class="
        variant === 'card' ? 'card card-hover p-4' : 'cursor-pointer px-3 py-2 hover:bg-ink-700'
      "
      :role="variant === 'compact' ? 'option' : undefined"
      @click="onSelect(item)"
    >
      <RouterLink v-if="variant === 'card'" :to="item.url" class="flex items-start gap-3">
        <span class="mt-0.5 shrink-0 rounded bg-ink-700 px-1.5 py-0.5 text-[10px] text-text-muted">
          {{ typeLabel(item.type) }}
        </span>
        <div class="min-w-0 flex-1">
          <h2 class="mb-1 font-medium text-text-primary hover:text-grass-300">
            <template v-for="(seg, i) in titleSegments(item)" :key="i">
              <mark v-if="seg.match" class="bg-grass-500/30 text-grass-200">{{ seg.text }}</mark>
              <template v-else>{{ seg.text }}</template>
            </template>
          </h2>
          <p v-if="item.summary" class="line-clamp-2 text-sm text-text-secondary">
            <template v-for="(seg, i) in summarySegments(item)" :key="i">
              <mark v-if="seg.match" class="bg-grass-500/30 text-grass-200">{{ seg.text }}</mark>
              <template v-else>{{ seg.text }}</template>
            </template>
          </p>
        </div>
      </RouterLink>

      <template v-else>
        <div class="flex items-center gap-2">
          <span class="shrink-0 rounded bg-ink-700 px-1.5 py-0.5 text-[10px] text-text-muted">
            {{ typeLabel(item.type) }}
          </span>
          <span class="truncate text-sm text-text-primary">
            <template v-for="(seg, i) in titleSegments(item)" :key="i">
              <mark v-if="seg.match" class="bg-grass-500/30 text-grass-200">{{ seg.text }}</mark>
              <template v-else>{{ seg.text }}</template>
            </template>
          </span>
        </div>
        <p v-if="item.summary" class="mt-0.5 truncate pl-1 text-xs text-text-muted">
          <template v-for="(seg, i) in summarySegments(item)" :key="i">
            <mark v-if="seg.match" class="bg-grass-500/30 text-grass-200">{{ seg.text }}</mark>
            <template v-else>{{ seg.text }}</template>
          </template>
        </p>
      </template>
    </li>
  </ul>
</template>
