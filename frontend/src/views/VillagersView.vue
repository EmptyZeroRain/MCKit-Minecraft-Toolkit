<script setup lang="ts">
// 村民交易查询（TODO §9B）：按名称、职业类别、游戏版本检索村民职业，并展示等级通用规则。
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
import { getVillagerTradeLevels, listVillagers } from '@/api/public'
import {
  VILLAGER_KINDS,
  VILLAGER_KIND_LABELS,
  type VillagerKind,
  type VillagerProfession,
  type VillagerTradeLevel,
} from '@/types/villager'
import { ApiError } from '@/types/api'
import { useRecentToolsStore } from '@/stores/recent-tools'
import { applyPageMeta } from '@/utils/seo'
import { reportToolFailure } from '@/utils/tool-stats'
import { buildVersionOptions, loadReleaseVersions, majorVersions } from '@/utils/mc-versions'
import { buildShareUrl, queryNumber, queryString } from '@/utils/share'

applyPageMeta({
  title: '村民交易查询',
  description:
    '按职业与游戏版本查询 Minecraft 村民的交易表：职业工作站、交易等级（新手→大师）、库存、经验与价格浮动。',
  canonicalPath: '/villagers',
})

useRecentToolsStore().record('villagers', '村民交易', '查询类')

const route = useRoute()
const router = useRouter()

const filters = reactive<{
  q: string
  kind: VillagerKind | ''
  version: string
  page: number
  pageSize: number
}>({
  q: queryString(route.query.q),
  kind: (queryString(route.query.kind) as VillagerKind | '') || '',
  version: queryString(route.query.version),
  page: queryNumber(route.query.page, 1),
  pageSize: 15,
})

const professions = ref<VillagerProfession[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')

const levels = ref<VillagerTradeLevel[]>([])
const levelsError = ref('')

const kindOptions: FilterOption[] = VILLAGER_KINDS.map((k) => ({
  label: VILLAGER_KIND_LABELS[k],
  value: k,
}))
// 版本筛选项动态来源于官方正式版清单；后端按精确版本语义匹配，
// 故选择 1.20 可命中声明 1.14+ 的职业，但不会命中声明 1.21+ 的条目。
const versionOptions = ref<FilterOption[]>(buildVersionOptions([], { includeAll: true }))

async function loadVillagers() {
  loading.value = true
  error.value = ''
  try {
    const res = await listVillagers({
      q: filters.q || undefined,
      kind: filters.kind || undefined,
      version: filters.version || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    })
    professions.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败，请稍后重试'
    // 核心动作（检索）失败计入工具失败率（TODO §13）。
    reportToolFailure('villagers')
  } finally {
    loading.value = false
  }
}

async function loadLevels() {
  levelsError.value = ''
  try {
    levels.value = await getVillagerTradeLevels()
  } catch (err) {
    levelsError.value = err instanceof ApiError ? err.message : '职业等级加载失败'
  }
}

/** 村民职业详情页的可分享绝对链接（§14：不携带任何页面现有参数）。 */
function detailShareUrl(slug: string): string {
  return buildShareUrl(`/villagers/${slug}`)
}

function syncUrl() {
  const params: Record<string, string> = {}
  if (filters.q) params.q = filters.q
  if (filters.kind) params.kind = filters.kind
  if (filters.version) params.version = filters.version
  if (filters.page > 1) params.page = String(filters.page)
  router.replace({ path: '/villagers', query: params })
}

watch(
  () => [filters.kind, filters.version],
  () => {
    filters.page = 1
    syncUrl()
    loadVillagers()
  },
)
watch(
  () => filters.q,
  () => {
    filters.page = 1
    syncUrl()
    loadVillagers()
  },
)

function onPageChange(p: number) {
  filters.page = p
  syncUrl()
  loadVillagers()
}

function clearAll() {
  filters.q = ''
  filters.kind = ''
  filters.version = ''
  filters.page = 1
  syncUrl()
}

onMounted(loadVillagers)
onMounted(loadLevels)

// 版本筛选项动态加载：失败时内部回退兜底清单，不会让筛选不可用。
onMounted(async () => {
  const versions = await loadReleaseVersions()
  versionOptions.value = buildVersionOptions(majorVersions(versions), { includeAll: true })
})
</script>

<template>
  <PageContainer maxWidth="max-w-6xl">
    <Breadcrumbs :items="[{ label: '村民交易' }]" />
    <div class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">村民交易</h1>
        <p class="text-text-secondary">
          查询村民职业的工作站、交易等级与交易表；数据来源为 Minecraft Wiki，并标注适用版本。
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
        aria-label="搜索村民职业"
        placeholder="例如：盔甲匠 / armorer / 高炉"
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
        <FilterPanel v-model="filters.kind" title="职业类别" :options="kindOptions" clearable />
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
        <!-- 职业等级通用规则（规则存于后端数据库，不硬编码） -->
        <section v-if="levels.length" class="card p-4" aria-labelledby="villager-levels-title">
          <h2 id="villager-levels-title" class="mb-2 text-sm font-semibold text-text-primary">职业等级</h2>
          <p class="mb-3 text-xs text-text-muted">
            村民通过交易累积经验升级；等级越高解锁的交易越强，价格也会随交易次数上涨。
          </p>
          <ul class="grid gap-2 text-sm sm:grid-cols-2 lg:grid-cols-3">
            <li v-for="level in levels" :key="level.id" class="rounded-md border border-border-dark p-3">
              <div class="flex items-baseline justify-between gap-2">
                <span class="font-medium text-text-primary">{{ level.name_zh }}</span>
                <span class="text-xs text-text-muted">{{ level.required_experience }} 经验</span>
              </div>
              <p class="mt-1 text-xs text-text-secondary">{{ level.badge_note }}</p>
            </li>
          </ul>
        </section>
        <ErrorState v-else-if="levelsError" :message="levelsError" retryable @retry="loadLevels" />

        <LoadingSkeleton v-if="loading" :count="5" />
        <ErrorState v-else-if="error" :message="error" retryable @retry="loadVillagers" />

        <div v-else-if="professions.length" class="card overflow-hidden">
          <div class="overflow-x-auto">
            <table class="w-full text-left text-sm">
              <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
                <tr>
                  <th scope="col" class="px-4 py-3 font-medium">职业</th>
                  <th scope="col" class="px-4 py-3 font-medium">类别</th>
                  <th scope="col" class="hidden px-4 py-3 font-medium md:table-cell">工作站</th>
                  <th scope="col" class="hidden px-4 py-3 font-medium lg:table-cell">适用版本</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-border-dark">
                <tr v-for="profession in professions" :key="profession.id" class="hover:bg-ink-800/60">
                  <td class="px-4 py-3">
                    <RouterLink
                      :to="{ name: 'villager-detail', params: { slug: profession.slug } }"
                      class="font-medium text-grass-300 hover:underline"
                    >
                      {{ profession.name_zh }}
                    </RouterLink>
                    <div v-if="profession.name_en" class="text-xs text-text-muted">{{ profession.name_en }}</div>
                    <p class="mt-1 text-xs text-text-secondary">{{ profession.summary }}</p>
                    <CopyButton
                      class="mt-1"
                      tool="villagers"
                      :text="detailShareUrl(profession.slug)"
                      label="复制链接"
                      :success-message="`已复制「${profession.name_zh}」详情链接`"
                    />
                  </td>
                  <td class="px-4 py-3 text-text-secondary">
                    {{ VILLAGER_KIND_LABELS[profession.kind] }}
                  </td>
                  <td class="hidden px-4 py-3 text-text-primary md:table-cell">
                    <template v-if="profession.workstation_name_zh">
                      {{ profession.workstation_name_zh }}
                      <div class="font-mono text-xs text-text-muted">{{ profession.workstation_item_id }}</div>
                    </template>
                    <span v-else class="text-text-muted">—</span>
                  </td>
                  <td class="hidden px-4 py-3 text-xs text-text-muted lg:table-cell">
                    {{ profession.supported_versions.join('、') || '—' }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <EmptyState
          v-else
          title="未找到匹配的村民职业"
          description="本站只收录已核验的村民交易资料；试试更换关键词或清除部分筛选条件。"
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
