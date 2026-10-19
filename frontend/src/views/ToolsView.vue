<script setup lang="ts">
// 工具中心（需求 5.2）：分类筛选、关键词搜索、工具卡片
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import ShareButton from '@/components/common/ShareButton.vue'
import FilterPanel, { type FilterOption } from '@/components/common/FilterPanel.vue'
import ToolCard from '@/components/cards/ToolCard.vue'
import BuiltinToolCard from '@/components/cards/BuiltinToolCard.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import Pagination from '@/components/common/Pagination.vue'
import { listTools } from '@/api/public'
import { ApiError } from '@/types/api'
import { TOOL_CATEGORIES, type ToolCategory } from '@/types/common'
import type { Tool } from '@/types/tool'
import { BUILTIN_TOOLS } from '@/utils/builtin-tools'
import { applyPageMeta } from '@/utils/seo'

applyPageMeta({
  title: '工具中心',
  description: 'Minecraft 工具中心：查询、计算、生成、编辑、辅助类在线工具。',
  canonicalPath: '/tools',
})

const route = useRoute()
const router = useRouter()

// 筛选状态同步到 URL query（需求 9.1）
const filters = reactive({
  category: (route.query.category as string) || '',
  q: (route.query.q as string) || '',
  page: Number(route.query.page) || 1,
  pageSize: 12,
}) as {
  category: ToolCategory | ''
  q: string
  page: number
  pageSize: number
}

const pageTitle = computed(() => '工具中心')

// 立即收藏工具卡，便于首页连续使用
const tools = ref<Tool[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')

const categoryOptions: FilterOption[] = TOOL_CATEGORIES.map((c) => ({ label: c, value: c }))

async function loadTools() {
  loading.value = true
  error.value = ''
  try {
    const res = await listTools({
      category: filters.category || undefined,
      q: filters.q || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    })
    tools.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

// 同步 URL（支持刷新与分享）
function syncUrl() {
  const query: Record<string, string> = {}
  if (filters.category) query.category = filters.category
  if (filters.q) query.q = filters.q
  if (filters.page > 1) query.page = String(filters.page)
  router.replace({ path: '/tools', query })
}

watch(
  () => filters.category,
  () => {
    filters.page = 1
    syncUrl()
    loadTools()
  },
)
watch(
  () => filters.q,
  () => {
    filters.page = 1
    syncUrl()
    loadTools()
  },
)

function onPageChange(p: number) {
  filters.page = p
  syncUrl()
  loadTools()
}

function clearSearch() {
  filters.q = ''
}

onMounted(loadTools)
</script>

<template>
  <PageContainer>
    <Breadcrumbs :items="[{ label: '工具中心' }]" />
    <div class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">{{ pageTitle }}</h1>
        <p class="text-text-secondary">前端内置的常用工具，以及后台维护的工具库。</p>

      </div>
      <ShareButton />
    </div>

    <!-- 内置工具：前端自带页面，导航不再单独占位，统一从这里进入 -->
    <section class="mb-8">
      <h2 class="mb-3 text-lg font-bold text-text-primary">内置工具</h2>
      <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <BuiltinToolCard v-for="tool in BUILTIN_TOOLS" :key="tool.slug" :tool="tool" />
      </div>
    </section>

    <h2 class="mb-3 text-lg font-bold text-text-primary">工具库</h2>

    <!-- 搜索栏 -->
    <div class="mb-6 flex flex-wrap items-center gap-3">
      <div class="relative flex-1 min-w-60">
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
          :aria-label="'搜索工具'"
          placeholder="按名称、标签搜索工具…"
          class="h-10 w-full rounded-md border border-border-dark bg-ink-800 pl-9 pr-9 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          @keydown.enter.prevent
        />
        <button
          v-if="filters.q"
          class="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted hover:text-text-primary"
          aria-label="清除搜索"
          @click="clearSearch"
        >
          ✕
        </button>
      </div>
    </div>

    <div class="grid gap-6 lg:grid-cols-4">
      <!-- 筛选侧栏 -->
      <aside class="lg:col-span-1">
        <FilterPanel v-model="filters.category" title="工具分类" :options="categoryOptions" clearable />
        <div class="mt-4 rounded-card border border-border-dark p-4 text-xs leading-relaxed text-text-secondary">
          收藏与最近使用仅保存在本地浏览器，不会上传。
        </div>
      </aside>

      <!-- 工具列表 -->
      <div class="lg:col-span-3">
        <LoadingSkeleton v-if="loading" :count="6" />
        <ErrorState v-else-if="error" :message="error" retryable @retry="loadTools" />
        <template v-else-if="tools.length">
          <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            <ToolCard v-for="tool in tools" :key="tool.id" :tool="tool" />
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
          title="未找到匹配的工具"
          description="尝试更换关键词或清除分类筛选"
          action-label="清除筛选"
          @action="clearSearch"
        />
      </div>
    </div>
  </PageContainer>
</template>