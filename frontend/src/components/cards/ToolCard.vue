<script setup lang="ts">
// 工具卡片
import { RouterLink } from 'vue-router'
import type { Tool } from '@/types/tool'
import FavoriteButton from '@/components/common/FavoriteButton.vue'
import { useRecentToolsStore } from '@/stores/recent-tools'
import { formatVersionLabel } from '@/utils/format'

const props = defineProps<{ tool: Tool }>()

const recent = useRecentToolsStore()

function recordUse() {
  recent.record(props.tool.slug, props.tool.name, props.tool.category)
}
</script>

<template>
  <article class="card card-hover relative flex flex-col p-5">
    <div class="mb-3 flex items-start justify-between gap-2">
      <div class="flex items-center gap-3">
        <div
          class="flex h-11 w-11 items-center justify-center rounded-lg border border-border-light bg-ink-700 text-xl"
          aria-hidden="true"
        >
          {{ tool.icon || '🧰' }}
        </div>
        <div>
          <RouterLink
            :to="`/tools/${tool.slug}`"
            class="font-semibold text-text-primary hover:text-grass-300"
            @click="recordUse"
          >
            {{ tool.name }}
          </RouterLink>
          <p class="text-xs text-text-muted">{{ tool.category }}</p>
        </div>
      </div>
      <FavoriteButton kind="tool" :slug="tool.slug" />
    </div>

    <p class="mb-4 flex-1 text-sm leading-relaxed text-text-secondary">
      {{ tool.summary }}
    </p>

    <div class="flex flex-wrap items-center gap-1.5">
      <span
        v-for="tag in tool.tags.slice(0, 4)"
        :key="tag"
        class="rounded bg-ink-700 px-1.5 py-0.5 text-xs text-text-muted"
      >
        #{{ tag }}
      </span>
      <span
        v-if="tool.supported_versions.length"
        class="ml-auto text-xs text-text-muted"
      >
        {{ tool.supported_versions.slice(0, 3).map(formatVersionLabel).join(' / ') }}
      </span>
    </div>
  </article>
</template>