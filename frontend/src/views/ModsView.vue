<script setup lang="ts">
// Mod 与整合包兼容性查询（TODO §16）：双 tab 检索。
//   Mod tab：按名称、加载器、类别、适用游戏版本检索 Mod，展示加载器、类别与适用版本；
//   整合包 tab：按名称、加载器、目标游戏版本检索整合包，展示目标版本与作者。
// 仅展示已发布（published）内容；依赖 / 冲突只呈现**已记录**关系，本站不做自动冲突分析。
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
import { listMods, listModpacks } from '@/api/public'
import {
  MOD_CATEGORIES,
  MOD_CATEGORY_LABELS,
  MOD_LOADER_LABELS,
  MOD_LOADERS,
  categoryClass,
  loaderClass,
  type Mod,
  type ModCategory,
  type ModLoader,
  type Modpack,
} from '@/types/mods'
import { ApiError } from '@/types/api'
import { useRecentToolsStore } from '@/stores/recent-tools'
import { applyPageMeta } from '@/utils/seo'
import { reportToolFailure } from '@/utils/tool-stats'
import { buildVersionOptions, loadReleaseVersions, majorVersions } from '@/utils/mc-versions'
import { buildShareUrl, queryNumber, queryString } from '@/utils/share'

applyPageMeta({
  title: 'Mod 与整合包兼容性',
  description:
    '查询 Minecraft Mod 的加载器、适用版本、前置依赖与冲突记录，以及整合包的包内 Mod 清单；只呈现已记录并标注来源与验证状态的兼容性信息。',
  canonicalPath: '/mods',
})

useRecentToolsStore().record('mods', 'Mod 兼容性', '查询类')

type ModsTab = 'mods' | 'modpacks'

const route = useRoute()
const router = useRouter()

const filters = reactive<{
  tab: ModsTab
  q: string
  loader: ModLoader | ''
  category: ModCategory | ''
  version: string
  page: number
  pageSize: number
}>({
  tab: queryString(route.query.tab) === 'modpacks' ? 'modpacks' : 'mods',
  q: queryString(route.query.q),
  loader: (queryString(route.query.loader) as ModLoader | '') || '',
  category: (queryString(route.query.category) as ModCategory | '') || '',
  version: queryString(route.query.version),
  page: queryNumber(route.query.page, 1),
  pageSize: 12,
})

const mods = ref<Mod[]>([])
const modpacks = ref<Modpack[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')

const loaderOptions: FilterOption[] = MOD_LOADERS.map((l) => ({
  label: MOD_LOADER_LABELS[l],
  value: l,
}))
const categoryOptions: FilterOption[] = MOD_CATEGORIES.map((c) => ({
  label: MOD_CATEGORY_LABELS[c],
  value: c,
}))

// Mod 的适用版本按区间语义（如 1.14+）匹配，使用主版本即可命中；
// 整合包的目标版本由后端精确匹配，故使用完整正式版清单，避免选 1.20 命不中 1.20.1。
const modVersionOptions = ref<FilterOption[]>(buildVersionOptions([], { includeAll: true }))
const packVersionOptions = ref<FilterOption[]>(buildVersionOptions([]))

const isMods = computed(() => filters.tab === 'mods')

const versionOptions = computed(() =>
  isMods.value ? modVersionOptions.value : packVersionOptions.value,
)

const placeholder = computed(() =>
  isMods.value ? '例如：JEI / 优化 / fabric-api' : '例如：All the Mods / 整合包 / ATM9',
)

async function loadList() {
  loading.value = true
  error.value = ''
  try {
    if (isMods.value) {
      const res = await listMods({
        q: filters.q || undefined,
        loader: filters.loader || undefined,
        category: filters.category || undefined,
        version: filters.version || undefined,
        page: filters.page,
        page_size: filters.pageSize,
      })
      mods.value = res.items
      total.value = res.total
      totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
    } else {
      const res = await listModpacks({
        q: filters.q || undefined,
        loader: filters.loader || undefined,
        version: filters.version || undefined,
        page: filters.page,
        page_size: filters.pageSize,
      })
      modpacks.value = res.items
      total.value = res.total
      totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
    }
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败，请稍后重试'
    // 核心动作（检索）失败计入工具失败率（TODO §13）。
    reportToolFailure('mods')
  } finally {
    loading.value = false
  }
}

/** 详情页的可分享绝对链接（§14：不携带任何页面现有参数）。 */
function modShareUrl(slug: string): string {
  return buildShareUrl(`/mods/${slug}`)
}

function modpackShareUrl(slug: string): string {
  return buildShareUrl(`/modpacks/${slug}`)
}

function syncUrl() {
  const params: Record<string, string> = {}
  if (filters.tab === 'modpacks') params.tab = 'modpacks'
  if (filters.q) params.q = filters.q
  if (filters.loader) params.loader = filters.loader
  if (filters.category) params.category = filters.category
  if (filters.version) params.version = filters.version
  if (filters.page > 1) params.page = String(filters.page)
  router.replace({ path: '/mods', query: params })
}

function switchTab(tab: ModsTab) {
  if (filters.tab === tab) return
  filters.tab = tab
  filters.page = 1
  // 两 tab 的游戏版本筛选语义不同（Mod 按适用版本区间、整合包按目标版本精确匹配），
  // 切换时清空，避免残留无效版本导致空结果。
  filters.version = ''
  syncUrl()
  loadList()
}

watch(
  () => [filters.loader, filters.category, filters.version],
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
  filters.loader = ''
  filters.category = ''
  filters.version = ''
  filters.page = 1
  syncUrl()
  loadList()
}

onMounted(loadList)
// 版本筛选项动态加载：失败时内部回退兜底清单，不会让筛选不可用。
onMounted(async () => {
  const versions = await loadReleaseVersions()
  modVersionOptions.value = buildVersionOptions(majorVersions(versions), { includeAll: true })
  packVersionOptions.value = buildVersionOptions(versions)
})
</script>

<template>
  <PageContainer maxWidth="max-w-6xl">
    <Breadcrumbs :items="[{ label: 'Mod 与整合包' }]" />
    <div class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">Mod 与整合包兼容性</h1>
        <p class="text-text-secondary">
          查询 Mod 的加载器、适用版本、前置依赖与冲突记录，以及整合包的包内 Mod
          清单；只呈现已记录并标注来源与验证状态的兼容性信息，不做自动冲突分析。
        </p>
      </div>
      <ShareButton />
    </div>

    <!-- tab 切换 -->
    <div class="mb-6 flex gap-2 border-b border-border-dark" role="tablist">
      <button
        type="button"
        role="tab"
        :aria-selected="isMods"
        class="border-b-2 px-4 py-2 text-sm font-medium transition"
        :class="
          isMods
            ? 'border-grass-500 text-text-primary'
            : 'border-transparent text-text-muted hover:text-text-primary'
        "
        @click="switchTab('mods')"
      >
        Mod
      </button>
      <button
        type="button"
        role="tab"
        :aria-selected="!isMods"
        class="border-b-2 px-4 py-2 text-sm font-medium transition"
        :class="
          !isMods
            ? 'border-grass-500 text-text-primary'
            : 'border-transparent text-text-muted hover:text-text-primary'
        "
        @click="switchTab('modpacks')"
      >
        整合包
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
        :aria-label="isMods ? '搜索 Mod' : '搜索整合包'"
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
        <FilterPanel v-model="filters.loader" title="加载器" :options="loaderOptions" clearable />
        <FilterPanel
          v-if="isMods"
          v-model="filters.category"
          title="类别"
          :options="categoryOptions"
          clearable
        />
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

        <!-- Mod 列表 -->
        <div v-else-if="isMods && mods.length" class="grid gap-4 sm:grid-cols-2">
          <article v-for="mod in mods" :key="mod.id" class="card flex flex-col p-4">
            <div class="mb-2 flex flex-wrap items-center gap-2">
              <span
                v-for="loader in mod.loaders"
                :key="loader"
                class="rounded border px-2 py-0.5 text-xs"
                :class="loaderClass(loader)"
              >
                {{ MOD_LOADER_LABELS[loader] }}
              </span>
              <span
                v-for="category in mod.categories"
                :key="category"
                class="rounded border px-2 py-0.5 text-xs"
                :class="categoryClass(category)"
              >
                {{ MOD_CATEGORY_LABELS[category] }}
              </span>
            </div>

            <h2 class="text-base font-semibold">
              <RouterLink
                :to="{ name: 'mod-detail', params: { slug: mod.slug } }"
                class="text-grass-300 hover:underline"
              >
                {{ mod.name_zh }}
              </RouterLink>
              <span v-if="mod.name_en" class="ml-2 text-xs font-normal text-text-muted">
                {{ mod.name_en }}
              </span>
            </h2>

            <p class="mt-1 flex-1 text-sm text-text-secondary">{{ mod.summary }}</p>

            <dl class="mt-3 text-xs text-text-muted">
              <div>
                <dt class="inline">适用版本：</dt>
                <dd class="inline text-text-secondary">{{ mod.mc_versions.join('、') || '—' }}</dd>
              </div>
            </dl>

            <CopyButton
              class="mt-2 self-start"
              tool="mods"
              :text="modShareUrl(mod.slug)"
              label="复制链接"
              :success-message="`已复制「${mod.name_zh}」详情链接`"
            />
          </article>
        </div>

        <!-- 整合包列表 -->
        <div v-else-if="!isMods && modpacks.length" class="grid gap-4 sm:grid-cols-2">
          <article v-for="pack in modpacks" :key="pack.id" class="card flex flex-col p-4">
            <div class="mb-2 flex flex-wrap items-center gap-2">
              <span
                v-if="pack.loader"
                class="rounded border px-2 py-0.5 text-xs"
                :class="loaderClass(pack.loader)"
              >
                {{ MOD_LOADER_LABELS[pack.loader] }}
              </span>
              <span
                v-if="pack.mc_version"
                class="rounded border border-border-light bg-ink-700 px-2 py-0.5 text-xs text-text-muted"
              >
                {{ pack.mc_version }}
              </span>
            </div>

            <h2 class="text-base font-semibold">
              <RouterLink
                :to="{ name: 'modpack-detail', params: { slug: pack.slug } }"
                class="text-grass-300 hover:underline"
              >
                {{ pack.name_zh }}
              </RouterLink>
              <span v-if="pack.name_en" class="ml-2 text-xs font-normal text-text-muted">
                {{ pack.name_en }}
              </span>
            </h2>

            <p class="mt-1 flex-1 text-sm text-text-secondary">{{ pack.summary }}</p>

            <dl class="mt-3 text-xs text-text-muted">
              <div>
                <dt class="inline">作者：</dt>
                <dd class="inline text-text-secondary">{{ pack.author || '—' }}</dd>
              </div>
            </dl>

            <CopyButton
              class="mt-2 self-start"
              tool="mods"
              :text="modpackShareUrl(pack.slug)"
              label="复制链接"
              :success-message="`已复制「${pack.name_zh}」详情链接`"
            />
          </article>
        </div>

        <EmptyState
          v-else
          :title="isMods ? '未找到匹配的 Mod' : '未找到匹配的整合包'"
          description="本站只收录已核验并发布的内容；试试更换关键词或清除部分筛选条件。"
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
