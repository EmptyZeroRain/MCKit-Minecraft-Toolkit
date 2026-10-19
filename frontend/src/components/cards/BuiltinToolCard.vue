<script setup lang="ts">
// 内置工具卡片：前端自带页面（非后台工具表数据），
// 首页快捷入口与工具中心「内置工具」区共用，保证展示与链接一致。
import { RouterLink } from 'vue-router'
import type { BuiltinTool } from '@/utils/builtin-tools'
import { useRecentToolsStore } from '@/stores/recent-tools'

const props = defineProps<{ tool: BuiltinTool }>()

const recent = useRecentToolsStore()

function recordUse() {
  recent.record(props.tool.slug, props.tool.name, props.tool.category)
}
</script>

<template>
  <RouterLink
    :to="tool.route"
    class="card card-hover flex items-center gap-3 p-4"
    @click="recordUse"
  >
    <span
      class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-border-light bg-ink-700 text-lg"
      aria-hidden="true"
    >
      {{ tool.icon }}
    </span>
    <span class="min-w-0">
      <span class="block font-medium text-text-primary">{{ tool.name }}</span>
      <span class="block truncate text-xs text-text-muted">{{ tool.summary }}</span>
    </span>
  </RouterLink>
</template>
