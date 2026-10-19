<script setup lang="ts">
// 开发文档知识库（TODO §21）：按名称、平台、主题与游戏版本检索模组 / 数据包开发文档。
import { onMounted, reactive, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import ShareButton from '@/components/common/ShareButton.vue'
import FilterPanel, { type FilterOption } from '@/components/common/FilterPanel.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import Pagination from '@/components/common/Pagination.vue'
import CopyButton from '@/components/common/CopyButton.vue'
import { getDevDocFacets, listDevDocs } from '@/api/dev'
import {
  DEV_LOADERS,
  DEV_CATEGORIES,
  devCategoryLabel,
  devLoaderLabel,
  type DevCategory,
  type DevDocSummary,
  type DevLoader,
} from '@/types/devdoc'
import { ApiError } from '@/types/api'
import { useRecentToolsStore } from '@/stores/recent-tools'
import { applyPageMeta } from '@/utils/seo'
import { reportToolFailure } from '@/utils/tool-stats'
import { formatVersionList } from '@/utils/format'
import { buildVersionOptions, loadReleaseVersions, majorVersions } from '@/utils/mc-versions'
import { buildShareUrl, queryNumber, queryString } from '@/utils/share'

applyPageMeta({
  title: '我的世界开发文档',
  description:
    'NeoForge、Forge、Fabric 与原版数据包开发知识库：入门与项目结构、事件、注册表、网络、数据生成、序列化等，附关键接口速查与官方文档链接。',
  canonicalPath: '/dev',
})

useRecentToolsStore().record('dev-docs', '开发文档', '查询类')

const route = useRoute()
const router = useRouter()

const filters = reactive<{
  q: string
  loader: DevLoader | ''
  category: DevCategory | ''
  version: string
  page: number
  pageSize: number
}>({
  q: queryString(route.query.q),
  loader: (queryString(route.query.loader) as DevLoader | '') || '',
  category: (queryString(route.query.category) as DevCategory | '') || '',
  version: queryString(route.query.version),
  page: queryNumber(route.query.page, 1),
  pageSize: 12,
})

const docs = ref<DevDocSummary[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')

// 筛选项优先取后端统计（含计数），失败时回退到本地枚举，保证筛选始终可用。
const loaderOptions = ref<FilterOption[]>(
  DEV_LOADERS.map((l) => ({ label: devLoaderLabel(l), value: l })),
)
const categoryOptions = ref<FilterOption[]>(
  DEV_CATEGORIES.map((c) => ({ label: devCategoryLabel(c), value: c })),
)
const versionOptions = ref<FilterOption[]>(buildVersionOptions([], { includeAll: true }))

async function loadFacets() {
  try {
    const facets = await getDevDocFacets()
    if (facets.loaders.length) {
      loaderOptions.value = facets.loaders.map((f) => ({
        label: `${devLoaderLabel(f.value)}（${f.count}）`,
        value: f.value,
      }))
    }
    if (facets.categories.length) {
      categoryOptions.value = facets.categories.map((f) => ({
        label: `${devCategoryLabel(f.value)}（${f.count}）`,
        value: f.value,
      }))
    }
  } catch {
    // 统计失败不阻断主流程：筛选项已由本地枚举兜底。
  }
}

async function loadDocs() {
  loading.value = true
  error.value = ''
  try {
    const res = await listDevDocs({
      q: filters.q || undefined,
      loader: filters.loader || undefined,
      category: filters.category || undefined,
      version: filters.version || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    })
    docs.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败，请稍后重试'
    // 核心动作（检索）失败计入工具失败率（TODO §13）。
    reportToolFailure('dev-docs')
  } finally {
    loading.value = false
  }
}

/** 详情页可分享绝对链接（§14：只覆盖查询/筛选类工具，不携带现有参数）。 */
function detailShareUrl(slug: string): string {
  return buildShareUrl(`/dev/${slug}`)
}

function syncUrl() {
  const params: Record<string, string> = {}
  if (filters.q) params.q = filters.q
  if (filters.loader) params.loader = filters.loader
  if (filters.category) params.category = filters.category
  if (filters.version) params.version = filters.version
  if (filters.page > 1) params.page = String(filters.page)
  router.replace({ path: '/dev', query: params })
}

watch(
  () => [filters.loader, filters.category, filters.version],
  () => {
    filters.page = 1
    syncUrl()
    loadDocs()
  },
)
watch(
  () => filters.q,
  () => {
    filters.page = 1
    syncUrl()
    loadDocs()
  },
)

function onPageChange(p: number) {
  filters.page = p
  syncUrl()
  loadDocs()
}

function clearAll() {
  filters.q = ''
  filters.loader = ''
  filters.category = ''
  filters.version = ''
  filters.page = 1
  syncUrl()
  loadDocs()
}

onMounted(loadDocs)
onMounted(loadFacets)
onMounted(async () => {
  const versions = await loadReleaseVersions()
  versionOptions.value = buildVersionOptions(majorVersions(versions), { includeAll: true })
})
</script>

<template>
  <PageContainer maxWidth="max-w-6xl">
    <Breadcrumbs :items="[{ label: '开发文档' }]" />
    <div class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">我的世界开发文档</h1>
        <p class="text-text-secondary">
          NeoForge、Forge、Fabric 与原版数据包开发知识库；每条包含要点提炼、关键接口速查与官方文档链接。
        </p>
      </div>
      <ShareButton />
    </div>

    <!-- 搜索 -->
    <div class="relative mb-6 max-w-xl">
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
        aria-label="搜索开发文档"
        placeholder="例如：事件总线 / DeferredRegister / Mixin / Codec"
        class="h-11 w-full rounded-md border border-border-dark bg-ink-800 pl-9 pr-9 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
      />
      <button
        v-if="filters.q"
        class="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted hover:text-text-primary"
        aria-label="清除"
        @click="filters.q = ''"
      >
        ✕
      </button>
    </div>

    <div class="grid gap-6 lg:grid-cols-4">
      <aside class="space-y-4 lg:col-span-1">
        <FilterPanel v-model="filters.loader" title="平台 / 加载器" :options="loaderOptions" clearable />
        <FilterPanel v-model="filters.category" title="主题" :options="categoryOptions" clearable />
        <FilterPanel v-model="filters.version" title="游戏版本" :options="versionOptions" clearable />
        <button
          type="button"
          class="w-full rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
          @click="clearAll"
        >
          清除全部筛选
        </button>
      </aside>

      <div class="space-y-6 lg:col-span-3">
        <p v-if="!loading && !error" class="text-xs text-text-muted">
          共 {{ total }} 条；版本筛选按「声明适用范围」精确匹配（选 1.21 会命中声明 1.20.1+ 的条目）。
        </p>

        <LoadingSkeleton v-if="loading" :count="4" />
        <ErrorState v-else-if="error" :message="error" retryable @retry="loadDocs" />

        <div v-else-if="docs.length" class="grid gap-4 md:grid-cols-2">
          <article v-for="doc in docs" :key="doc.slug" class="card card-hover flex flex-col p-5">
            <div class="mb-2 flex flex-wrap items-center gap-2 text-xs">
              <span class="rounded border border-border-light bg-ink-700 px-2 py-0.5 text-text-secondary">
                {{ devLoaderLabel(doc.loader) }}
              </span>
              <span class="text-text-muted">{{ devCategoryLabel(doc.category) }}</span>
            </div>
            <h2 class="mb-1 text-base font-semibold text-text-primary">
              <RouterLink
                :to="{ name: 'dev-doc-detail', params: { slug: doc.slug } }"
                class="hover:text-grass-300 hover:underline"
              >
                {{ doc.title_zh }}
              </RouterLink>
            </h2>
            <p v-if="doc.title_en" class="mb-2 text-xs text-text-muted">{{ doc.title_en }}</p>
            <p class="mb-3 line-clamp-3 text-sm text-text-secondary">{{ doc.summary }}</p>

            <div class="mt-auto space-y-2">
              <div class="flex flex-wrap gap-1.5">
                <span
                  v-for="tag in doc.tags.slice(0, 4)"
                  :key="tag"
                  class="rounded bg-ink-700 px-1.5 py-0.5 text-xs text-text-muted"
                >
                  #{{ tag }}
                </span>
              </div>
              <div class="flex flex-wrap items-center justify-between gap-2 text-xs text-text-muted">
                <span>{{ formatVersionList(doc.mc_versions, 2) }}</span>
                <span v-if="doc.api_entry_count">{{ doc.api_entry_count }} 个关键接口</span>
              </div>
              <CopyButton
                tool="dev-docs"
                :text="detailShareUrl(doc.slug)"
                label="复制链接"
                :success-message="`已复制「${doc.title_zh}」链接`"
              />
            </div>
          </article>
        </div>

        <EmptyState
          v-else
          title="未找到匹配的开发文档"
          description="试试更换关键词，或清除平台 / 主题 / 版本筛选。"
          action-label="清除筛选"
          @action="clearAll"
        />

        <Pagination
          :page="filters.page"
          :page-size="filters.pageSize"
          :total="total"
          :total-pages="totalPages"
          @change="onPageChange"
        />
      </div>
    </div>
  </PageContainer>
</template>
