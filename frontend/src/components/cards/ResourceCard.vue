<script setup lang="ts">
// 资源卡片：封面、名称、摘要、作者、类型、版本、加载器、平台
import { RouterLink } from 'vue-router'
import type { Resource } from '@/types/resource'
import ResourceTypeBadge from '@/components/badges/ResourceTypeBadge.vue'
import VersionBadge from '@/components/badges/VersionBadge.vue'
import LoaderBadge from '@/components/badges/LoaderBadge.vue'
import FavoriteButton from '@/components/common/FavoriteButton.vue'
import { timeAgo } from '@/utils/format'

defineProps<{ resource: Resource }>()
</script>

<template>
  <article class="card card-hover flex flex-col overflow-hidden">
    <RouterLink
      :to="`/resources/${resource.slug}`"
      class="relative block aspect-[16/7] w-full overflow-hidden bg-ink-700"
    >
      <img
        v-if="resource.cover_image_url"
        :src="resource.cover_image_url"
        :alt="resource.name"
        class="h-full w-full object-cover"
        loading="lazy"
      />
      <img
        v-else
        src="/placeholder-cover.svg"
        :alt="resource.name"
        class="h-full w-full object-cover"
        loading="lazy"
      />
      <span class="absolute left-3 top-3">
        <ResourceTypeBadge :type="resource.resource_type" />
      </span>
    </RouterLink>

    <div class="flex flex-1 flex-col p-4">
      <div class="mb-1 flex items-start justify-between gap-2">
        <RouterLink
          :to="`/resources/${resource.slug}`"
          class="font-semibold leading-snug text-text-primary hover:text-grass-300"
        >
          {{ resource.name }}
        </RouterLink>
        <FavoriteButton kind="resource" :slug="resource.slug" />
      </div>
      <p class="mb-1 text-xs text-text-muted">
        {{ resource.author_name || '未知作者' }}
        <span v-if="resource.source_platform"> · {{ resource.source_platform }}</span>
      </p>
      <p class="mb-3 line-clamp-2 flex-1 text-sm leading-relaxed text-text-secondary">
        {{ resource.summary }}
      </p>

      <div class="flex flex-wrap items-center gap-1.5">
        <VersionBadge
          v-for="v in resource.supported_versions.slice(0, 3)"
          :key="v"
          :version="v"
        />
        <LoaderBadge
          v-for="l in resource.loader_types.slice(0, 2)"
          :key="l"
          :loader="l"
        />
      </div>

      <div class="mt-3 flex items-center justify-between border-t border-border-dark pt-2 text-xs text-text-muted">
        <span>更新于 {{ timeAgo(resource.updated_at) }}</span>
        <span
          class="inline-flex items-center gap-1 text-grass-400"
        >
          查看详情
          <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M13.5 4.5L21 12l-7.5 7.5M21 12H3" />
          </svg>
        </span>
      </div>
    </div>
  </article>
</template>