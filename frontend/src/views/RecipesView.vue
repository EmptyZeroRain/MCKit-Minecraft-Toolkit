<script setup lang="ts">
// 合成配方查询（需求 5.5）
import { onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import ShareButton from '@/components/common/ShareButton.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import Pagination from '@/components/common/Pagination.vue'
import { listRecipes } from '@/api/public'
import type { Recipe } from '@/types/recipe'
import { RECIPE_TYPES, RECIPE_TYPE_LABELS, type RecipeType } from '@/types/common'
import { ApiError } from '@/types/api'
import { useRecentToolsStore } from '@/stores/recent-tools'
import { applyPageMeta } from '@/utils/seo'
import { reportToolUse } from '@/utils/tool-stats'
import { resolveIngredients, resolveResultItem, type RelatedItem } from '@/utils/item-link'
import { loadReleaseVersions } from '@/utils/mc-versions'

applyPageMeta({
  title: '合成配方查询',
  description: '查询 Minecraft 物品的合成配方，支持版本筛选与配方类型展示。',
  canonicalPath: '/recipes',
})

// 记录最近使用
useRecentToolsStore().record('recipes', '合成配方查询', '查询类')

const route = useRoute()
const router = useRouter()

const query = reactive<{
  item_id: string
  game_version: string
  recipe_type: RecipeType | ''
  page: number
  pageSize: number
}>({
  item_id: (route.query.item_id as string) || '',
  game_version: (route.query.game_version as string) || '最新',
  recipe_type: (route.query.recipe_type as RecipeType | '') || '',
  page: 1,
  pageSize: 15,
})

const recipes = ref<Recipe[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')

// 游戏版本选项（TODO §6-C）：来自官方正式版清单（动态覆盖全部 MC 版本）；
// 「最新」表示不传版本参数，由后端返回最新版本数据。
const versionOptions = ref<string[]>(['最新'])

async function loadRecipes() {
  loading.value = true
  error.value = ''
  try {
    const res = await listRecipes({
      item_id: query.item_id || undefined,
      game_version: query.game_version === '最新' ? undefined : query.game_version,
      recipe_type: query.recipe_type || undefined,
      page: query.page,
      page_size: query.pageSize,
    })
    recipes.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / query.pageSize))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

/** 用户主动点击「查询」：记一次工具使用（TODO §13），再执行查询。 */
function onSearch() {
  reportToolUse('recipes')
  void loadRecipes()
}

function syncUrl() {
  const params: Record<string, string> = {}
  if (query.item_id) params.item_id = query.item_id
  if (query.game_version && query.game_version !== '最新') params.game_version = query.game_version
  if (query.recipe_type) params.recipe_type = query.recipe_type
  router.replace({ path: '/recipes', query: params })
}

watch(
  () => [query.item_id, query.game_version, query.recipe_type],
  () => {
    query.page = 1
    syncUrl()
    loadRecipes()
  },
)

function onPageChange(p: number) {
  query.page = p
  loadRecipes()
}

function clearAll() {
  query.item_id = ''
  query.game_version = '最新'
  query.recipe_type = ''
}

onMounted(loadRecipes)

// 版本选项动态加载（失败内部回退兜底清单）。
onMounted(async () => {
  const versions = await loadReleaseVersions()
  versionOptions.value = ['最新', ...versions]
})

// ===== 配方网格渲染 =====
function gridRows(recipe: Recipe): string[][] {
  if (recipe.pattern?.length) {
    return recipe.pattern.map((row) => row.split('').map((c) => c.trim()))
  }
  // 无序合成或数据缺失：以 ingredients 生成单列表/示意
  const flat = recipe.ingredients ?? []
  const fill: string[] = []
  for (let i = 0; i < 9; i++) fill.push(flat[i]?.item_id ?? '')
  const rows: string[][] = []
  for (let r = 0; r < 3; r++) rows.push(fill.slice(r * 3, r * 3 + 3))
  return rows
}

function keyToItem(recipe: Recipe, key: string): string {
  if (!key) return ''
  return recipe.key_mapping?.[key] ?? key
}

function gridCellLabel(recipe: Recipe, char: string): string {
  return keyToItem(recipe, char)
}

function shortItemId(itemId: string): string {
  const idx = itemId.indexOf(':')
  return idx >= 0 ? itemId.slice(idx + 1) : itemId
}

function recipeTypeLabel(t: string): string {
  return RECIPE_TYPE_LABELS[t as (typeof RECIPE_TYPES)[number]] ?? t
}

function onGridItemClick(itemId: string) {
  if (!itemId) return
  query.item_id = itemId
  query.page = 1
  loadRecipes()
}

// ===== 关联物品解析（TODO §3）=====
// 结果物品与材料是否可点击，取决于后端是否确认该物品已收录（resolved_items）。
// 用 WeakMap 缓存，避免模板重复计算。
interface RecipeRelated {
  result: RelatedItem
  ingredients: Array<RelatedItem & { count: number }>
}
const relatedCache = new WeakMap<Recipe, RecipeRelated>()

function related(recipe: Recipe): RecipeRelated {
  let cached = relatedCache.get(recipe)
  if (!cached) {
    cached = { result: resolveResultItem(recipe), ingredients: resolveIngredients(recipe) }
    relatedCache.set(recipe, cached)
  }
  return cached
}
</script>

<template>
  <PageContainer maxWidth="max-w-6xl">
    <Breadcrumbs :items="[{ label: '合成配方查询' }]" />
    <div class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">合成配方查询</h1>
        <p class="text-text-secondary">输入目标物品名称或 ID，查看其合成/烧炼配方。</p>

      </div>
      <ShareButton />
    </div>

    <!-- 筛选栏 -->
    <div class="card mb-6 flex flex-wrap items-end gap-4 p-4">
      <div class="min-w-56 flex-1">
        <label class="mb-1 block text-xs font-medium text-text-muted" for="recipe-item">目标物品（名称 / ID）</label>
        <input
          id="recipe-item"
          v-model="query.item_id"
          type="search"
          placeholder="例如：crafting_table / 工作台"
          class="h-10 w-full rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
        />
      </div>
      <div>
        <label class="mb-1 block text-xs font-medium text-text-muted" for="recipe-ver">游戏版本</label>
        <select
          id="recipe-ver"
          v-model="query.game_version"
          class="h-10 rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
        >
          <option v-for="v in versionOptions" :key="v" :value="v">{{ v }}</option>
        </select>
      </div>
      <button
        type="button"
        class="h-10 rounded-md bg-grass-600 px-5 text-sm font-medium text-white transition hover:bg-grass-500"
        @click="onSearch"
      >
        查询
      </button>
      <button
        type="button"
        class="h-10 text-sm text-text-muted transition hover:text-text-primary"
        @click="clearAll"
      >
        重置
      </button>
    </div>

    <LoadingSkeleton v-if="loading" :count="4" />
    <ErrorState v-else-if="error" :message="error" retryable @retry="loadRecipes" />

    <div v-else-if="recipes.length" class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
      <article v-for="recipe in recipes" :key="recipe.id" class="card p-4">
        <div class="mb-3 flex items-center justify-between">
          <div class="flex items-center gap-2">
            <RouterLink
              v-if="related(recipe).result.url"
              :to="related(recipe).result.url as string"
              class="font-mono text-sm text-grass-300 hover:underline"
              :title="`查看 ${related(recipe).result.itemId} 的资料`"
            >
              {{ related(recipe).result.label }}
            </RouterLink>
            <span
              v-else
              class="font-mono text-sm text-text-secondary"
              :title="`${recipe.result_item_id}（未收录物品资料）`"
            >
              {{ related(recipe).result.label }}
            </span>
            <span v-if="recipe.result_count > 1" class="rounded bg-grass-500/15 px-1.5 text-xs text-grass-300">
              ×{{ recipe.result_count }}
            </span>
          </div>
          <span class="rounded border border-border-light bg-ink-700 px-1.5 py-0.5 text-xs text-text-muted">
            {{ recipeTypeLabel(recipe.recipe_type) }}
          </span>
        </div>

        <div class="mb-3 flex justify-center">
          <div class="grid grid-cols-3 gap-0.5 rounded-md border border-border-dark bg-ink-950 p-0.5">
            <template v-for="(row, ri) in gridRows(recipe)" :key="ri">
              <div
                v-for="(cell, ci) in row"
                :key="`${ri}-${ci}`"
                class="flex h-12 w-12 items-center justify-center rounded-sm border border-border-dark bg-ink-700"
                :title="gridCellLabel(recipe, cell) || '空'"
              >
                <button
                  v-if="cell"
                  type="button"
                  class="text-[10px] leading-tight text-text-secondary transition hover:text-grass-300"
                  @click="onGridItemClick(gridCellLabel(recipe, cell))"
                >
                  {{ shortItemId(gridCellLabel(recipe, cell)) }}
                </button>
              </div>
            </template>
          </div>
        </div>

        <div class="flex flex-wrap gap-1.5">
          <template v-for="ing in related(recipe).ingredients" :key="ing.itemId">
            <RouterLink
              v-if="ing.url"
              :to="ing.url"
              class="rounded bg-ink-700 px-1.5 py-0.5 text-[10px] text-grass-300 transition hover:bg-ink-600"
            >
              {{ ing.label }}{{ ing.count > 1 ? ` ×${ing.count}` : '' }}
            </RouterLink>
            <span
              v-else
              class="rounded bg-ink-700 px-1.5 py-0.5 text-[10px] text-text-muted"
              :title="`${ing.itemId}（未收录物品资料）`"
            >
              {{ ing.label }}{{ ing.count > 1 ? ` ×${ing.count}` : '' }}
            </span>
          </template>
        </div>

        <div class="mt-3 flex flex-wrap justify-between text-xs text-text-muted">
          <span>{{ recipe.game_version || '—' }}</span>
          <span v-if="recipe.requires_crafting_table">需要工作台</span>
        </div>
      </article>
    </div>

    <EmptyState
      v-else
      title="没有找到该物品的配方"
      description="请检查物品名称拼写，或尝试更换游戏版本筛选。"
      action-label="重置筛选"
      @action="clearAll"
    />

    <div class="mt-8">
      <Pagination
        :page="query.page"
        :page-size="query.pageSize"
        :total="total"
        :total-pages="totalPages"
        @change="onPageChange"
      />
    </div>
  </PageContainer>
</template>