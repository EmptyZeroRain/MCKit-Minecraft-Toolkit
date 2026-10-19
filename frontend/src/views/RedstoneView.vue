<script setup lang="ts">
// 红石与机器设计资料库（TODO §15）：双 tab 检索。
//   元件 tab：按名称、元件类别、游戏版本检索红石元件，展示工作机制与信号说明；
//   设计 tab：按名称、用途、难度、游戏版本检索机器设计，仅展示已发布（published）内容。
// 状态与 URL 双向同步（§14），便于分享还原。
import { computed, onMounted, reactive, ref, watch } from 'vue'
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
import { listRedstoneComponents, listRedstoneDesigns } from '@/api/public'
import {
  REDSTONE_COMPONENT_CATEGORIES,
  REDSTONE_COMPONENT_CATEGORY_LABELS,
  REDSTONE_DESIGN_PURPOSES,
  REDSTONE_DESIGN_PURPOSE_LABELS,
  REDSTONE_DIFFICULTIES,
  REDSTONE_DIFFICULTY_LABELS,
  componentCategoryClass,
  designPurposeClass,
  difficultyClass,
  type RedstoneComponent,
  type RedstoneComponentCategory,
  type RedstoneDesign,
  type RedstoneDesignDifficulty,
  type RedstoneDesignPurpose,
} from '@/types/redstone'
import { ApiError } from '@/types/api'
import { useRecentToolsStore } from '@/stores/recent-tools'
import { applyPageMeta } from '@/utils/seo'
import { reportToolFailure } from '@/utils/tool-stats'
import { buildVersionOptions, loadReleaseVersions, majorVersions } from '@/utils/mc-versions'
import { buildShareUrl, queryNumber, queryString } from '@/utils/share'

applyPageMeta({
  title: '红石与机器设计资料',
  description:
    '查询 Minecraft 红石元件的工作机制与信号说明，以及机器设计的用途、难度、材料清单与搭建步骤；标注适用版本、来源与内容审核状态。',
  canonicalPath: '/redstone',
})

useRecentToolsStore().record('redstone', '红石资料', '查询类')

type RedstoneTab = 'components' | 'designs'

const route = useRoute()
const router = useRouter()

const filters = reactive<{
  tab: RedstoneTab
  q: string
  category: RedstoneComponentCategory | ''
  purpose: RedstoneDesignPurpose | ''
  difficulty: RedstoneDesignDifficulty | ''
  version: string
  page: number
  pageSize: number
}>({
  tab: queryString(route.query.tab) === 'designs' ? 'designs' : 'components',
  q: queryString(route.query.q),
  category: (queryString(route.query.category) as RedstoneComponentCategory | '') || '',
  purpose: (queryString(route.query.purpose) as RedstoneDesignPurpose | '') || '',
  difficulty: (queryString(route.query.difficulty) as RedstoneDesignDifficulty | '') || '',
  version: queryString(route.query.version),
  page: queryNumber(route.query.page, 1),
  pageSize: 12,
})

const components = ref<RedstoneComponent[]>([])
const designs = ref<RedstoneDesign[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')

const categoryOptions: FilterOption[] = REDSTONE_COMPONENT_CATEGORIES.map((c) => ({
  label: REDSTONE_COMPONENT_CATEGORY_LABELS[c],
  value: c,
}))
const purposeOptions: FilterOption[] = REDSTONE_DESIGN_PURPOSES.map((p) => ({
  label: REDSTONE_DESIGN_PURPOSE_LABELS[p],
  value: p,
}))
const difficultyOptions: FilterOption[] = REDSTONE_DIFFICULTIES.map((d) => ({
  label: REDSTONE_DIFFICULTY_LABELS[d],
  value: d,
}))
// 版本筛选项动态来源于官方正式版清单；后端按精确版本语义匹配，
// 故选择 1.20 可命中声明「1.14+」的条目，但不会命中声明「1.21+」的条目。
const versionOptions = ref<FilterOption[]>(buildVersionOptions([], { includeAll: true }))

const isComponents = computed(() => filters.tab === 'components')

const placeholder = computed(() =>
  isComponents.value ? '例如：红石粉 / redstone / 活塞' : '例如：甘蔗农场 / farm / 分类机',
)

async function loadList() {
  loading.value = true
  error.value = ''
  try {
    if (isComponents.value) {
      const res = await listRedstoneComponents({
        q: filters.q || undefined,
        category: filters.category || undefined,
        version: filters.version || undefined,
        page: filters.page,
        page_size: filters.pageSize,
      })
      components.value = res.items
      total.value = res.total
      totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
    } else {
      const res = await listRedstoneDesigns({
        q: filters.q || undefined,
        purpose: filters.purpose || undefined,
        difficulty: filters.difficulty || undefined,
        version: filters.version || undefined,
        page: filters.page,
        page_size: filters.pageSize,
      })
      designs.value = res.items
      total.value = res.total
      totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
    }
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败，请稍后重试'
    // 核心动作（检索）失败计入工具失败率（TODO §13）。
    reportToolFailure('redstone')
  } finally {
    loading.value = false
  }
}

/** 详情页的可分享绝对链接（§14：不携带任何页面现有参数）。 */
function componentShareUrl(slug: string): string {
  return buildShareUrl(`/redstone/components/${slug}`)
}

function designShareUrl(slug: string): string {
  return buildShareUrl(`/redstone/designs/${slug}`)
}

function syncUrl() {
  const params: Record<string, string> = {}
  if (filters.tab === 'designs') params.tab = 'designs'
  if (filters.q) params.q = filters.q
  if (filters.category) params.category = filters.category
  if (filters.purpose) params.purpose = filters.purpose
  if (filters.difficulty) params.difficulty = filters.difficulty
  if (filters.version) params.version = filters.version
  if (filters.page > 1) params.page = String(filters.page)
  router.replace({ path: '/redstone', query: params })
}

function switchTab(tab: RedstoneTab) {
  if (filters.tab === tab) return
  filters.tab = tab
  filters.page = 1
  syncUrl()
  loadList()
}

watch(
  () => [filters.category, filters.purpose, filters.difficulty, filters.version],
  () => {
    filters.page = 1
    syncUrl()
    loadList()
  },
)
watch(
  () => filters.q,
  () => {
    filters.page = 1
    syncUrl()
    loadList()
  },
)

function onPageChange(p: number) {
  filters.page = p
  syncUrl()
  loadList()
}

function clearAll() {
  filters.q = ''
  filters.category = ''
  filters.purpose = ''
  filters.difficulty = ''
  filters.version = ''
  filters.page = 1
  syncUrl()
  loadList()
}

onMounted(loadList)
// 版本筛选项动态加载：失败时内部回退兜底清单，不会让筛选不可用。
onMounted(async () => {
  const versions = await loadReleaseVersions()
  versionOptions.value = buildVersionOptions(majorVersions(versions), { includeAll: true })
})
</script>

<template>
  <PageContainer maxWidth="max-w-6xl">
    <Breadcrumbs :items="[{ label: '红石与机器设计' }]" />
    <div class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">红石与机器设计</h1>
        <p class="text-text-secondary">
          查询红石元件的工作机制与信号说明，以及机器设计的用途、难度、材料清单与搭建步骤；数据来源为 Minecraft
          Wiki，并标注适用版本与内容审核状态。
        </p>
      </div>
      <ShareButton />
    </div>

    <!-- tab 切换 -->
    <div class="mb-6 flex gap-2 border-b border-border-dark" role="tablist">
      <button
        type="button"
        role="tab"
        :aria-selected="isComponents"
        class="border-b-2 px-4 py-2 text-sm font-medium transition"
        :class="
          isComponents
            ? 'border-grass-500 text-text-primary'
            : 'border-transparent text-text-muted hover:text-text-primary'
        "
        @click="switchTab('components')"
      >
        红石元件
      </button>
      <button
        type="button"
        role="tab"
        :aria-selected="!isComponents"
        class="border-b-2 px-4 py-2 text-sm font-medium transition"
        :class="
          !isComponents
            ? 'border-grass-500 text-text-primary'
            : 'border-transparent text-text-muted hover:text-text-primary'
        "
        @click="switchTab('designs')"
      >
        机器设计
      </button>
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
        :aria-label="isComponents ? '搜索红石元件' : '搜索机器设计'"
        :placeholder="placeholder"
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
      <!-- 筛选侧栏 -->
      <aside class="space-y-4 lg:col-span-1">
        <template v-if="isComponents">
          <FilterPanel v-model="filters.category" title="元件类别" :options="categoryOptions" clearable />
        </template>
        <template v-else>
          <FilterPanel v-model="filters.purpose" title="设计用途" :options="purposeOptions" clearable />
          <FilterPanel v-model="filters.difficulty" title="难度" :options="difficultyOptions" clearable />
        </template>
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
        <LoadingSkeleton v-if="loading" :count="5" />
        <ErrorState v-else-if="error" :message="error" retryable @retry="loadList" />

        <!-- 元件列表 -->
        <div v-else-if="isComponents && components.length" class="grid gap-4 sm:grid-cols-2">
          <article v-for="component in components" :key="component.id" class="card flex flex-col p-4">
            <div class="mb-2 flex flex-wrap items-center gap-2">
              <span class="rounded border px-2 py-0.5 text-xs" :class="componentCategoryClass(component.category)">
                {{ REDSTONE_COMPONENT_CATEGORY_LABELS[component.category] }}
              </span>
            </div>

            <h2 class="text-base font-semibold">
              <RouterLink
                :to="{ name: 'redstone-component-detail', params: { slug: component.slug } }"
                class="text-grass-300 hover:underline"
              >
                {{ component.name_zh }}
              </RouterLink>
              <span v-if="component.name_en" class="ml-2 text-xs font-normal text-text-muted">
                {{ component.name_en }}
              </span>
            </h2>

            <p class="mt-1 flex-1 text-sm text-text-secondary">{{ component.summary }}</p>
            <p v-if="component.mechanism_note" class="mt-2 line-clamp-3 text-xs text-text-muted">
              {{ component.mechanism_note }}
            </p>

            <dl class="mt-3 text-xs text-text-muted">
              <div>
                <dt class="inline">适用版本：</dt>
                <dd class="inline text-text-secondary">{{ component.supported_versions.join('、') || '—' }}</dd>
              </div>
            </dl>

            <CopyButton
              class="mt-2 self-start"
              tool="redstone"
              :text="componentShareUrl(component.slug)"
              label="复制链接"
              :success-message="`已复制「${component.name_zh}」详情链接`"
            />
          </article>
        </div>

        <!-- 设计列表 -->
        <div v-else-if="!isComponents && designs.length" class="grid gap-4 sm:grid-cols-2">
          <article v-for="design in designs" :key="design.id" class="card flex flex-col p-4">
            <div class="mb-2 flex flex-wrap items-center gap-2">
              <span class="rounded border px-2 py-0.5 text-xs" :class="designPurposeClass(design.purpose)">
                {{ REDSTONE_DESIGN_PURPOSE_LABELS[design.purpose] }}
              </span>
              <span class="rounded border px-2 py-0.5 text-xs" :class="difficultyClass(design.difficulty)">
                难度：{{ REDSTONE_DIFFICULTY_LABELS[design.difficulty] }}
              </span>
            </div>

            <h2 class="text-base font-semibold">
              <RouterLink
                :to="{ name: 'redstone-design-detail', params: { slug: design.slug } }"
                class="text-grass-300 hover:underline"
              >
                {{ design.name_zh }}
              </RouterLink>
              <span v-if="design.name_en" class="ml-2 text-xs font-normal text-text-muted">
                {{ design.name_en }}
              </span>
            </h2>

            <p class="mt-1 flex-1 text-sm text-text-secondary">{{ design.summary }}</p>

            <dl class="mt-3 grid grid-cols-2 gap-2 text-xs text-text-muted">
              <div>
                <dt class="inline">材料：</dt>
                <dd class="inline text-text-secondary">{{ design.materials.length }} 种</dd>
              </div>
              <div>
                <dt class="inline">适用版本：</dt>
                <dd class="inline text-text-secondary">{{ design.supported_versions.join('、') || '—' }}</dd>
              </div>
            </dl>

            <CopyButton
              class="mt-2 self-start"
              tool="redstone"
              :text="designShareUrl(design.slug)"
              label="复制链接"
              :success-message="`已复制「${design.name_zh}」详情链接`"
            />
          </article>
        </div>

        <EmptyState
          v-else
          :title="isComponents ? '未找到匹配的红石元件' : '未找到匹配的机器设计'"
          :description="
            isComponents
              ? '本站只收录已核验的红石元件资料；试试更换关键词或清除部分筛选条件。'
              : '本站只展示已通过内容审核的机器设计；试试更换关键词或清除部分筛选条件。'
          "
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
