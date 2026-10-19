<script setup lang="ts">
// 生物资料查询（TODO §9C）：按名称、生物类别、游戏版本检索生物，
// 展示掉落、生成条件与驯服 / 繁殖方式。
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
import { listMobs } from '@/api/public'
import {
  MOB_CATEGORIES,
  MOB_CATEGORY_LABELS,
  MOB_DIMENSION_LABELS,
  dimensionClass,
  formatHealth,
  type Mob,
  type MobCategory,
} from '@/types/mob'
import { ApiError } from '@/types/api'
import { useRecentToolsStore } from '@/stores/recent-tools'
import { applyPageMeta } from '@/utils/seo'
import { reportToolFailure } from '@/utils/tool-stats'
import { buildVersionOptions, loadReleaseVersions, majorVersions } from '@/utils/mc-versions'
import { buildShareUrl, queryNumber, queryString } from '@/utils/share'

applyPageMeta({
  title: '生物资料查询',
  description:
    '按类别与游戏版本查询 Minecraft 生物资料：生命值、掉落物、生成条件、驯服与繁殖方式，并标注适用版本与来源。',
  canonicalPath: '/mobs',
})

useRecentToolsStore().record('mobs', '生物资料', '查询类')

const route = useRoute()
const router = useRouter()

const filters = reactive<{
  q: string
  category: MobCategory | ''
  version: string
  page: number
  pageSize: number
}>({
  q: queryString(route.query.q),
  category: (queryString(route.query.category) as MobCategory | '') || '',
  version: queryString(route.query.version),
  page: queryNumber(route.query.page, 1),
  pageSize: 12,
})

const mobs = ref<Mob[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')

const categoryOptions: FilterOption[] = MOB_CATEGORIES.map((c) => ({
  label: MOB_CATEGORY_LABELS[c],
  value: c,
}))
// 版本筛选项动态来源于官方正式版清单；后端按精确版本语义匹配，
// 故选择 1.20 可命中声明 1.14+ 的生物，但不会命中声明 1.21+ 的条目。
const versionOptions = ref<FilterOption[]>(buildVersionOptions([], { includeAll: true }))

async function loadMobs() {
  loading.value = true
  error.value = ''
  try {
    const res = await listMobs({
      q: filters.q || undefined,
      category: filters.category || undefined,
      version: filters.version || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    })
    mobs.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败，请稍后重试'
    // 核心动作（检索）失败计入工具失败率（TODO §13）。
    reportToolFailure('mobs')
  } finally {
    loading.value = false
  }
}

/** 生物详情页的可分享绝对链接（§14：不携带任何页面现有参数）。 */
function detailShareUrl(slug: string): string {
  return buildShareUrl(`/mobs/${slug}`)
}

function syncUrl() {
  const params: Record<string, string> = {}
  if (filters.q) params.q = filters.q
  if (filters.category) params.category = filters.category
  if (filters.version) params.version = filters.version
  if (filters.page > 1) params.page = String(filters.page)
  router.replace({ path: '/mobs', query: params })
}

watch(
  () => [filters.category, filters.version],
  () => {
    filters.page = 1
    syncUrl()
    loadMobs()
  },
)
watch(
  () => filters.q,
  () => {
    filters.page = 1
    syncUrl()
    loadMobs()
  },
)

function onPageChange(p: number) {
  filters.page = p
  syncUrl()
  loadMobs()
}

function clearAll() {
  filters.q = ''
  filters.category = ''
  filters.version = ''
  filters.page = 1
  syncUrl()
}

onMounted(loadMobs)
// 版本筛选项动态加载：失败时内部回退兜底清单，不会让筛选不可用。
onMounted(async () => {
  const versions = await loadReleaseVersions()
  versionOptions.value = buildVersionOptions(majorVersions(versions), { includeAll: true })
})
</script>

<template>
  <PageContainer maxWidth="max-w-6xl">
    <Breadcrumbs :items="[{ label: '生物资料' }]" />
    <div class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">生物资料</h1>
        <p class="text-text-secondary">
          查询生物的生命值、掉落物、生成条件与驯服 / 繁殖方式；数据来源为 Minecraft Wiki，并标注适用版本。
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
        aria-label="搜索生物"
        placeholder="例如：僵尸 / zombie / 末影人"
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
        <FilterPanel v-model="filters.category" title="生物类别" :options="categoryOptions" clearable />
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
        <ErrorState v-else-if="error" :message="error" retryable @retry="loadMobs" />

        <div v-else-if="mobs.length" class="grid gap-4 sm:grid-cols-2">
          <article v-for="mob in mobs" :key="mob.id" class="card flex flex-col p-4">
            <div class="mb-2 flex flex-wrap items-center gap-2">
              <span class="rounded border border-border-light bg-ink-700 px-2 py-0.5 text-xs text-text-muted">
                {{ MOB_CATEGORY_LABELS[mob.category] }}
              </span>
              <span
                v-for="dim in mob.dimensions"
                :key="dim"
                class="rounded border px-2 py-0.5 text-xs"
                :class="dimensionClass(dim)"
              >
                {{ MOB_DIMENSION_LABELS[dim] }}
              </span>
            </div>

            <h2 class="text-base font-semibold">
              <RouterLink
                :to="{ name: 'mob-detail', params: { slug: mob.slug } }"
                class="text-grass-300 hover:underline"
              >
                {{ mob.name_zh }}
              </RouterLink>
              <span v-if="mob.name_en" class="ml-2 text-xs font-normal text-text-muted">{{ mob.name_en }}</span>
            </h2>

            <p class="mt-1 flex-1 text-sm text-text-secondary">{{ mob.summary }}</p>

            <dl class="mt-3 grid grid-cols-2 gap-2 text-xs text-text-muted">
              <div>
                <dt class="inline">生命值：</dt>
                <dd class="inline text-text-secondary">{{ formatHealth(mob.health) }}</dd>
              </div>
              <div>
                <dt class="inline">适用版本：</dt>
                <dd class="inline text-text-secondary">{{ mob.supported_versions.join('、') || '—' }}</dd>
              </div>
            </dl>

            <CopyButton
              class="mt-2 self-start"
              tool="mobs"
              :text="detailShareUrl(mob.slug)"
              label="复制链接"
              :success-message="`已复制「${mob.name_zh}」详情链接`"
            />
          </article>
        </div>

        <EmptyState
          v-else
          title="未找到匹配的生物"
          description="本站只收录已核验的生物资料；试试更换关键词或清除部分筛选条件。"
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
