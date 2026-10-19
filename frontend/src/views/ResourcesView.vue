<script setup lang="ts">
// 资源中心（需求 5.7）：关键词、类型、版本、加载器、来源平台筛选
import { onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import ShareButton from '@/components/common/ShareButton.vue'
import FilterPanel, { type FilterOption } from '@/components/common/FilterPanel.vue'
import ResourceCard from '@/components/cards/ResourceCard.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import Pagination from '@/components/common/Pagination.vue'
import { listResources } from '@/api/public'
import type { Resource } from '@/types/resource'
import {
  RESOURCE_TYPES,
  RESOURCE_TYPE_LABELS,
  LOADER_TYPES,
  LOADER_LABELS,
  type ResourceType,
  type LoaderType,
} from '@/types/common'
import { ApiError } from '@/types/api'
import { applyPageMeta } from '@/utils/seo'
import { buildVersionOptions, loadReleaseVersions, majorVersions } from '@/utils/mc-versions'

applyPageMeta({
  title: '资源中心',
  description: 'Minecraft 资源导航：Mod、整合包、资源包、光影、存档、数据包、插件。',
  canonicalPath: '/resources',
})

const route = useRoute()
const router = useRouter()

const filters = reactive<{
  q: string
  type: ResourceType | ''
  gameVersion: string
  loader: LoaderType | ''
  sourcePlatform: string
  sort: 'updated_desc' | 'name_asc'
  page: number
  pageSize: number
}>({
  q: (route.query.q as string) || '',
  type: (route.query.type as ResourceType | '') || '',
  gameVersion: (route.query.game_version as string) || '',
  loader: (route.query.loader as LoaderType | '') || '',
  sourcePlatform: (route.query.source_platform as string) || '',
  sort: ((route.query.sort as string) as 'updated_desc' | 'name_asc') || 'updated_desc',
  page: Number(route.query.page) || 1,
  pageSize: 12,
})

const resources = ref<Resource[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')

const typeOptions: FilterOption[] = RESOURCE_TYPES.map((t) => ({ label: RESOURCE_TYPE_LABELS[t], value: t }))
const loaderOptions: FilterOption[] = LOADER_TYPES.map((l) => ({ label: LOADER_LABELS[l], value: l }))
// 游戏版本筛选（TODO §6-C）：主版本取自官方正式版清单（动态，覆盖全部 MC 版本），
// 并额外提供「全部 MC 版本」用于筛出**声明了通配（all）**的资源；
// 后端按声明文本匹配，故选择主版本（如 1.20）可同时命中 1.20.1 等具体声明。
const versionOptions = ref<FilterOption[]>(buildVersionOptions([], { includeAll: true }))

async function loadResources() {
  loading.value = true
  error.value = ''
  try {
    const res = await listResources({
      q: filters.q || undefined,
      type: filters.type || undefined,
      game_version: filters.gameVersion || undefined,
      loader: filters.loader || undefined,
      source_platform: filters.sourcePlatform || undefined,
      sort: filters.sort,
      page: filters.page,
      page_size: filters.pageSize,
    })
    resources.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

function syncUrl() {
  const params: Record<string, string> = {}
  if (filters.q) params.q = filters.q
  if (filters.type) params.type = filters.type
  if (filters.gameVersion) params.game_version = filters.gameVersion
  if (filters.loader) params.loader = filters.loader
  if (filters.sourcePlatform) params.source_platform = filters.sourcePlatform
  if (filters.sort !== 'updated_desc') params.sort = filters.sort
  if (filters.page > 1) params.page = String(filters.page)
  router.replace({ path: '/resources', query: params })
}

watch(
  () => [filters.type, filters.gameVersion, filters.loader, filters.sourcePlatform, filters.sort],
  () => {
    filters.page = 1
    syncUrl()
    loadResources()
  },
)
watch(
  () => filters.q,
  () => {
    filters.page = 1
    syncUrl()
    loadResources()
  },
)

function onPageChange(p: number) {
  filters.page = p
  syncUrl()
  loadResources()
}

function clearAll() {
  filters.q = ''
  filters.type = ''
  filters.gameVersion = ''
  filters.loader = ''
  filters.sourcePlatform = ''
  filters.sort = 'updated_desc'
  filters.page = 1
  syncUrl()
}

onMounted(loadResources)

// 版本筛选项动态加载：失败时内部回退兜底清单，不会让筛选不可用。
onMounted(async () => {
  const versions = await loadReleaseVersions()
  versionOptions.value = buildVersionOptions(majorVersions(versions), { includeAll: true })
})
</script>

<template>
  <PageContainer>
    <Breadcrumbs :items="[{ label: '资源中心' }]" />
    <div class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">资源中心</h1>
        <p class="text-text-secondary">
          资源仅做索引，点击进入详情可跳转至原始发布来源；本站不托管、不代理第三方文件。
        </p>

      </div>
      <ShareButton />
    </div>

    <!-- 搜索 + 排序 -->
    <div class="mb-6 flex flex-wrap items-center gap-3">
      <div class="relative min-w-60 flex-1">
        <svg
          class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-muted"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          stroke-width="2"
        >
          <path stroke-linecap="round" d="M21 21l-4.35-4.35M17 10.5a6.5 6.5 0 11-13 0 6.5 6.5 0 0113 0z" />
        </svg>
        <input
          v-model="filters.q"
          type="search"
          aria-label="搜索资源"
          placeholder="搜索资源名称、作者、标签…"
          class="h-10 w-full rounded-md border border-border-dark bg-ink-800 pl-9 pr-9 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
        />
        <button
          v-if="filters.q"
          class="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted hover:text-text-primary"
          aria-label="清除搜索"
          @click="filters.q = ''"
        >
          ✕
        </button>
      </div>
      <label class="flex items-center gap-2 text-sm text-text-secondary">
        来源平台
        <input
          v-model="filters.sourcePlatform"
          type="text"
          aria-label="来源平台"
          placeholder="如 CurseForge"
          class="h-10 w-40 rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
        />
      </label>
      <label class="flex items-center gap-2 text-sm text-text-secondary">
        排序
        <select
          v-model="filters.sort"
          class="h-10 rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
        >
          <option value="updated_desc">最近更新</option>
          <option value="name_asc">名称升序</option>
        </select>
      </label>
    </div>

    <div class="grid gap-6 lg:grid-cols-4">
      <!-- 筛选侧栏 -->
      <aside class="space-y-4 lg:col-span-1">
        <FilterPanel v-model="filters.type" title="资源类型" :options="typeOptions" clearable />
        <FilterPanel v-model="filters.gameVersion" title="游戏版本" :options="versionOptions" clearable />
        <FilterPanel v-model="filters.loader" title="加载器" :options="loaderOptions" clearable />
        <button
          type="button"
          class="w-full rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
          @click="clearAll"
        >
          清除全部筛选
        </button>
      </aside>

      <!-- 资源列表 -->
      <div class="lg:col-span-3">
        <LoadingSkeleton v-if="loading" :count="6" />
        <ErrorState v-else-if="error" :message="error" retryable @retry="loadResources" />
        <template v-else-if="resources.length">
          <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            <ResourceCard v-for="res in resources" :key="res.id" :resource="res" />
          </div>
          <div class="mt-8">
            <Pagination
              :page="filters.page"
              :page-size="filters.pageSize"
              :total="total"
              :total-pages="totalPages"
              @change="onPageChange"
            />
          </div>
        </template>
        <EmptyState
          v-else
          title="未找到匹配的资源"
          description="尝试更换关键词或清除部分筛选条件。"
          action-label="清除筛选"
          @action="clearAll"
        />
      </div>
    </div>
  </PageContainer>
</template>