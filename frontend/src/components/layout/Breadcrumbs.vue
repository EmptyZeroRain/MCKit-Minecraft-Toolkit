<script setup lang="ts">
// 面包屑导航
import { RouterLink } from 'vue-router'

export interface Crumb {
  label: string
  to?: string
}

defineProps<{
  items: Crumb[]
}>()
</script>

<template>
  <nav aria-label="面包屑" class="mb-4 text-sm text-text-muted">
    <ol class="flex flex-wrap items-center gap-1.5">
      <li>
        <RouterLink to="/" class="hover:text-grass-300">首页</RouterLink>
      </li>
      <template v-for="(item, idx) in items" :key="idx">
        <li aria-hidden="true" class="text-text-muted">/</li>
        <li>
          <RouterLink
            v-if="item.to && idx < items.length - 1"
            :to="item.to"
            class="hover:text-grass-300"
          >
            {{ item.label }}
          </RouterLink>
          <span v-else class="text-text-primary" aria-current="page">{{ item.label }}</span>
        </li>
      </template>
    </ol>
  </nav>
</template>