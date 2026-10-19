<script setup lang="ts">
// 搜索结果页（需求 5.1）：类型筛选 + 分页 + 空结果建议
import { onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import ShareButton from '@/components/common/ShareButton.vue'
import SearchResults from '@/components/search/SearchResults.vue'
import { search } from '@/api/public'
import type { SearchResultItem, SearchSort } from '@/types/search'
import EmptyState from '@/components/common/EmptyState.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import Pagination from '@/components/common/Pagination.vue'
import { ApiError } from '@/types/api'
import { applyPageMeta } from '@/utils/seo'
import { buildVersionOptions, loadReleaseVersions, majorVersions } from '@/utils/mc-versions'

const route = useRoute()
const router = useRouter()

const filters = reactive<{
  q: string
  type: '' | 'tool' | 'resource' | 'wiki' | 'item' | 'devdoc'
  gameVersion: string
  sort: SearchSort
  page: number
  pageSize: number
}>({
  q: (route.query.q as string) || '',
  type: (route.query.type as '' | 'tool' | 'resource' | 'wiki' | 'item' | 'devdoc') || '',
  gameVersion: (route.query.game_version as string) || '',
  sort: route.query.sort === 'updated_desc' ? 'updated_desc' : 'relevance',
  page: Number(route.query.page) || 1,
  pageSize: 15,
})

const results = ref<SearchResultItem[]>([])
/** 无结果时后端给出的可尝试关键词（TODO §5，基于已记录搜索词的字面关系）。 */
const suggestions = ref<string[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')

const typeOptions: Array<{ label: string; value: '' | 'tool' | 'resource' | 'wiki' | 'item' | 'devdoc' }> = [
  { label: '全部', value: '' },
  { label: '工具', value: 'tool' },
  { label: '资源', value: 'resource' },
  { label: '百科', value: 'wiki' },
  { label: '物品', value: 'item' },
  { label: '开发文档', value: 'devdoc' },
]

const sortOptions: Array<{ label: string; value: SearchSort }> = [
  { label: '相关度', value: 'relevance' },
  { label: '最近更新', value: 'updated_desc' },
]

/** 游戏版本筛选（TODO §5 / §6-C）；空值表示不限版本。
 *  选项来自官方正式版清单（动态覆盖全部 MC 版本），并含「全部 MC 版本」通配筛选。 */
const versionOptions = ref<{ label: string; value: string }[]>([])

async function loadResults() {
  if (!filters.q.trim()) return
  loading.value = true
  error.value = ''
  try {
    const res = await search({
      q: filters.q,
      type: filters.type || undefined,
      game_version: filters.gameVersion || undefined,
      sort: filters.sort,
      page: filters.page,
      page_size: filters.pageSize,
    })
    results.value = res.items
    suggestions.value = res.suggestions
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
    applyPageMeta({
      title: `搜索结果：${filters.q}`,
      description: 'Minecraft 工具、资源、百科、物品与开发文档全站搜索结果。',
      robot: 'noindex,nofollow',
    })
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '搜索失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

function syncUrl() {
  const params: Record<string, string> = { q: filters.q }
  if (filters.type) params.type = filters.type
  if (filters.gameVersion) params.game_version = filters.gameVersion
  if (filters.sort === 'updated_desc') params.sort = filters.sort
  if (filters.page > 1) params.page = String(filters.page)
  router.replace({ path: '/search', query: params })
}

/** 用建议关键词重搜（TODO §5 无结果引导）。 */
function applySuggestion(term: string) {
  filters.q = term
  filters.page = 1
  syncUrl()
  loadResults()
}

watch(
  () => filters.type,
  () => {
    filters.page = 1
    syncUrl()
    loadResults()
  },
)

watch(
  () => filters.sort,
  () => {
    filters.page = 1
    syncUrl()
    loadResults()
  },
)

watch(
  () => filters.gameVersion,
  () => {
    filters.page = 1
    syncUrl()
    loadResults()
  },
)

function onPageChange(p: number) {
  filters.page = p
  syncUrl()
  loadResults()
}

onMounted(loadResults)

// 版本选项动态加载（失败内部回退兜底清单）。
onMounted(async () => {
  const versions = await loadReleaseVersions()
  versionOptions.value = buildVersionOptions(majorVersions(versions), { includeAll: true })
})
</script>

<template>
  <PageContainer maxWidth="max-w-5xl">
    <Breadcrumbs :items="[{ label: '搜索结果' }]" />

    <div class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">
          搜索“<span class="text-grass-400">{{ filters.q || '—' }}</span>”
        </h1>
        <p v-if="total > 0" class="text-sm text-text-muted">共找到 {{ total }} 条结果</p>

      </div>
      <ShareButton />
    </div>

    <div v-if="filters.q.trim()">
      <!-- 类型筛选 -->
      <div class="mb-3 flex flex-wrap gap-2">
        <button
          v-for="opt in typeOptions"
          :key="opt.value"
          type="button"
          class="rounded-full border px-3 py-1 text-xs transition"
          :class="
            filters.type === opt.value
              ? 'border-grass-500 bg-grass-500/15 text-grass-300'
              : 'border-border-dark bg-ink-700 text-text-secondary hover:bg-ink-600'
          "
          @click="filters.type = opt.value"
        >
          {{ opt.label }}
        </button>
      </div>

      <!-- 排序（TODO §5：相关度 / 最近更新）与版本筛选 -->
      <div class="mb-5 flex flex-wrap items-center gap-3 text-xs">
        <div class="flex flex-wrap items-center gap-2">
          <span class="text-text-muted">排序：</span>
          <button
            v-for="opt in sortOptions"
            :key="opt.value"
            type="button"
            class="rounded border px-2 py-0.5 transition"
            :class="
              filters.sort === opt.value
                ? 'border-grass-500 text-grass-300'
                : 'border-border-dark text-text-secondary hover:text-text-primary'
            "
            @click="filters.sort = opt.value"
          >
            {{ opt.label }}
          </button>
        </div>
        <div class="flex items-center gap-2">
          <label class="text-text-muted" for="search-version">游戏版本：</label>
          <select
            id="search-version"
            v-model="filters.gameVersion"
            class="h-8 rounded-md border border-border-dark bg-ink-800 px-2 text-xs text-text-primary focus:border-grass-500 focus:outline-none"
          >
            <option value="">不限</option>
            <option v-for="v in versionOptions" :key="v.value" :value="v.value">{{ v.label }}</option>
          </select>
        </div>
      </div>

      <LoadingSkeleton v-if="loading" :count="5" />
      <ErrorState v-else-if="error" :message="error" retryable @retry="loadResults" />

      <SearchResults v-else-if="results.length" :items="results" :keyword="filters.q" />

      <div v-else-if="!loading && !error" class="space-y-4">
        <EmptyState
          title="没有找到相关结果"
          description="建议：检查关键词拼写、尝试更短的关键词，或浏览资源中心与工具中心。"
          action-label="浏览资源中心"
          @action="router.push('/resources')"
        />

        <!-- TODO §5：无结果时给出可尝试的关键词（来自已记录的相近搜索词） -->
        <div v-if="suggestions.length" class="rounded-card border border-border-dark bg-ink-800 p-4">
          <p class="mb-2 text-sm text-text-muted">换个关键词试试：</p>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="term in suggestions"
              :key="term"
              type="button"
              class="rounded-full border border-border-dark bg-ink-700 px-3 py-1 text-xs text-text-secondary transition hover:border-grass-500 hover:text-grass-300"
              @click="applySuggestion(term)"
            >
              {{ term }}
            </button>
          </div>
        </div>
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
    </div>
  </PageContainer>
</template>