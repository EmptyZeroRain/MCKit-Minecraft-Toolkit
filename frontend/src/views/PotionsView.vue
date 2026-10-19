<script setup lang="ts">
// 药水与酿造查询（TODO §9A）：按名称、类别、游戏版本检索药水，并展示通用酿造规则。
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
import { getBrewingRules, listPotions } from '@/api/public'
import {
  POTION_CATEGORIES,
  POTION_CATEGORY_LABELS,
  potionEffectSummary,
  type BrewingRules,
  type Potion,
  type PotionCategory,
} from '@/types/potion'
import type { ItemRef } from '@/types/item'
import { resolveRelatedItem } from '@/utils/item-link'
import { ApiError } from '@/types/api'
import { useRecentToolsStore } from '@/stores/recent-tools'
import { applyPageMeta } from '@/utils/seo'
import { reportToolFailure } from '@/utils/tool-stats'
import { buildVersionOptions, loadReleaseVersions, majorVersions } from '@/utils/mc-versions'
import { buildShareUrl, queryNumber, queryString } from '@/utils/share'

applyPageMeta({
  title: '药水与酿造查询',
  description:
    '按名称、类别与游戏版本查询 Minecraft 药水的效果、持续时间与酿造步骤，含普通/延长/增强三种变体。',
  canonicalPath: '/potions',
})

useRecentToolsStore().record('potions', '药水与酿造', '查询类')

const route = useRoute()
const router = useRouter()

const filters = reactive<{
  q: string
  category: PotionCategory | ''
  version: string
  page: number
  pageSize: number
}>({
  q: queryString(route.query.q),
  category: (queryString(route.query.category) as PotionCategory | '') || '',
  version: queryString(route.query.version),
  page: queryNumber(route.query.page, 1),
  pageSize: 15,
})

const potions = ref<Potion[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')

const rules = ref<BrewingRules | null>(null)
const rulesError = ref('')

const categoryOptions: FilterOption[] = POTION_CATEGORIES.map((c) => ({
  label: POTION_CATEGORY_LABELS[c],
  value: c,
}))
// 版本筛选项的动态来源为官方正式版清单；后端按**精确版本语义**匹配，
// 故选择 1.20 可命中声明 1.8+ 的药水，但不会命中 1.21+ 的药水。
const versionOptions = ref<FilterOption[]>(buildVersionOptions([], { includeAll: true }))

async function loadPotions() {
  loading.value = true
  error.value = ''
  try {
    const res = await listPotions({
      q: filters.q || undefined,
      category: filters.category || undefined,
      version: filters.version || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    })
    potions.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败，请稍后重试'
    // 核心动作（检索）失败计入工具失败率（TODO §13）。
    reportToolFailure('potions')
  } finally {
    loading.value = false
  }
}

async function loadRules() {
  rulesError.value = ''
  try {
    rules.value = await getBrewingRules()
  } catch (err) {
    rulesError.value = err instanceof ApiError ? err.message : '酿造规则加载失败'
  }
}

/** 解析原料物品摘要：未收录时 url 为 null，页面只展示纯文本 ID（不生成死链）。 */
function ingredientLink(id: string) {
  return resolveRelatedItem(rules.value?.ingredient_items as Record<string, ItemRef> | undefined, id)
}

/** 药水详情页的可分享绝对链接（§14：不携带任何页面现有参数）。 */
function detailShareUrl(slug: string): string {
  return buildShareUrl(`/potions/${slug}`)
}

function syncUrl() {
  const params: Record<string, string> = {}
  if (filters.q) params.q = filters.q
  if (filters.category) params.category = filters.category
  if (filters.version) params.version = filters.version
  if (filters.page > 1) params.page = String(filters.page)
  router.replace({ path: '/potions', query: params })
}

watch(
  () => [filters.category, filters.version],
  () => {
    filters.page = 1
    syncUrl()
    loadPotions()
  },
)
watch(
  () => filters.q,
  () => {
    filters.page = 1
    syncUrl()
    loadPotions()
  },
)

function onPageChange(p: number) {
  filters.page = p
  syncUrl()
  loadPotions()
}

function clearAll() {
  filters.q = ''
  filters.category = ''
  filters.version = ''
  filters.page = 1
  syncUrl()
}

onMounted(loadPotions)
onMounted(loadRules)

// 版本筛选项动态加载：失败时内部回退兜底清单，不会让筛选不可用。
onMounted(async () => {
  const versions = await loadReleaseVersions()
  versionOptions.value = buildVersionOptions(majorVersions(versions), { includeAll: true })
})
</script>

<template>
  <PageContainer maxWidth="max-w-6xl">
    <Breadcrumbs :items="[{ label: '药水与酿造' }]" />
    <div class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">药水与酿造</h1>
        <p class="text-text-secondary">
          查询药水的效果、持续时间与酿造步骤；数据来源为 Minecraft Wiki，并标注适用版本。
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
        aria-label="搜索药水"
        placeholder="例如：治疗 / healing / 迅捷"
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
        <FilterPanel v-model="filters.category" title="药水类别" :options="categoryOptions" clearable />
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
        <!-- 通用酿造规则（规则存于后端数据库，不硬编码） -->
        <section v-if="rules?.rules.length" class="card p-4" aria-labelledby="brewing-rules-title">
          <h2 id="brewing-rules-title" class="mb-2 text-sm font-semibold text-text-primary">通用酿造规则</h2>
          <p class="mb-3 text-xs text-text-muted">
            以下原料可作用于任意符合条件的药水；所有酿造操作每次耗时 20 秒，需以烈焰粉为燃料。
          </p>
          <ul class="space-y-2 text-sm">
            <li v-for="rule in rules.rules" :key="rule.id" class="flex flex-wrap items-baseline gap-x-2">
              <span class="font-medium text-text-primary">{{ rule.ingredient_name_zh }}</span>
              <RouterLink
                v-if="ingredientLink(rule.ingredient_item_id).url"
                :to="ingredientLink(rule.ingredient_item_id).url as string"
                class="text-xs text-grass-300 hover:underline"
              >
                物品详情
              </RouterLink>
              <span v-else class="font-mono text-xs text-text-muted">
                {{ rule.ingredient_item_id }}
              </span>
              <span class="text-text-secondary">— {{ rule.note }}</span>
            </li>
          </ul>
        </section>
        <ErrorState v-else-if="rulesError" :message="rulesError" retryable @retry="loadRules" />

        <LoadingSkeleton v-if="loading" :count="5" />
        <ErrorState v-else-if="error" :message="error" retryable @retry="loadPotions" />

        <div v-else-if="potions.length" class="card overflow-hidden">
          <div class="overflow-x-auto">
            <table class="w-full text-left text-sm">
              <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
                <tr>
                  <th scope="col" class="px-4 py-3 font-medium">药水</th>
                  <th scope="col" class="px-4 py-3 font-medium">类别</th>
                  <th scope="col" class="px-4 py-3 font-medium">效果（普通）</th>
                  <th scope="col" class="hidden px-4 py-3 font-medium md:table-cell">变体</th>
                  <th scope="col" class="hidden px-4 py-3 font-medium lg:table-cell">适用版本</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-border-dark">
                <tr v-for="potion in potions" :key="potion.id" class="hover:bg-ink-800/60">
                  <td class="px-4 py-3">
                    <RouterLink
                      :to="{ name: 'potion-detail', params: { slug: potion.slug } }"
                      class="font-medium text-grass-300 hover:underline"
                    >
                      {{ potion.name_zh }}
                    </RouterLink>
                    <div v-if="potion.name_en" class="text-xs text-text-muted">{{ potion.name_en }}</div>
                    <CopyButton
                      class="mt-1"
                      tool="potions"
                      :text="detailShareUrl(potion.slug)"
                      label="复制链接"
                      :success-message="`已复制「${potion.name_zh}」详情链接`"
                    />
                  </td>
                  <td class="px-4 py-3 text-text-secondary">
                    {{ POTION_CATEGORY_LABELS[potion.category] }}
                  </td>
                  <td class="px-4 py-3 text-text-primary">{{ potionEffectSummary(potion) }}</td>
                  <td class="hidden px-4 py-3 md:table-cell">
                    <span v-if="potion.is_extendable" class="mr-1 text-xs text-text-secondary">可延长</span>
                    <span v-if="potion.is_enhanceable" class="text-xs text-text-secondary">可增强</span>
                    <span v-if="!potion.is_extendable && !potion.is_enhanceable" class="text-xs text-text-muted">—</span>
                  </td>
                  <td class="hidden px-4 py-3 text-xs text-text-muted lg:table-cell">
                    {{ potion.supported_versions.join('、') || '—' }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <EmptyState
          v-else
          title="未找到匹配的药水"
          description="本站只收录已核验的药水资料；试试更换关键词或清除部分筛选条件。"
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
