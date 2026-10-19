<script setup lang="ts">
// 物品详情（统一物品资料库，TODO §1）
// 展示物品资料、获取方式、用途、生成式获取指令与数据驱动的关联配方。
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import CopyButton from '@/components/common/CopyButton.vue'
import FavoriteButton from '@/components/common/FavoriteButton.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import VersionBadge from '@/components/badges/VersionBadge.vue'
import { getItem } from '@/api/public'
import type { ItemDetail } from '@/types/item'
import type { Recipe } from '@/types/recipe'
import { RECIPE_TYPE_LABELS, type RecipeType } from '@/types/common'
import { ApiError } from '@/types/api'
import { formatDateTime } from '@/utils/format'
import { applyPageMeta } from '@/utils/seo'
import { buildItemSearchLinks, itemCategoryLabel, supportsDurability } from '@/utils/item-category'
import { resolveRelatedItem, type RelatedItem } from '@/utils/item-link'

const route = useRoute()
const router = useRouter()

const itemId = computed(() => (route.params.itemId as string) || '')
const detail = ref<ItemDetail | null>(null)
const loading = ref(true)
const error = ref('')

const searchLinks = computed(() => (detail.value ? buildItemSearchLinks(detail.value) : null))
const showDurability = computed(() => supportsDurability(detail.value?.item_category))
const produced = computed(() => detail.value?.related_recipes?.produced ?? [])
const usedAs = computed(() => detail.value?.related_recipes?.used_as ?? [])
const hasRecipes = computed(() => produced.value.length + usedAs.value.length > 0)

// 关联配方引用到的、已收录物品的摘要（TODO §3）；未收录物品不在其中。
const resolvedRefs = computed(() => detail.value?.related_recipes?.resolved_items ?? {})

/** 解析单个 item_id 的展示信息（未收录时回退为纯文本，不生成链接）。 */
function itemLink(id: string): RelatedItem {
  return resolveRelatedItem(resolvedRefs.value, id)
}

/** 配方材料 + 数量（材料是否可跳转取决于是否已收录）。 */
function ingredientLinks(recipe: Recipe): Array<RelatedItem & { count: number }> {
  return (recipe.ingredients ?? []).map((ing) => ({
    ...resolveRelatedItem(resolvedRefs.value, ing.item_id),
    count: ing.count,
  }))
}

async function loadDetail() {
  loading.value = true
  error.value = ''
  detail.value = null
  try {
    const data = await getItem(itemId.value)
    detail.value = data
    if (data) {
      applyPageMeta({
        title: `${data.name_zh}（${data.item_id}）`,
        description: data.description || `${data.name_zh} 的物品资料：获取方式、用途与关联配方。`,
        canonicalPath: `/items/${data.item_id}`,
      })
    }
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

onMounted(loadDetail)
watch(itemId, loadDetail)

function shortItemId(id: string): string {
  const idx = id.indexOf(':')
  return idx >= 0 ? id.slice(idx + 1) : id
}

function recipeTypeLabel(t: string): string {
  return RECIPE_TYPE_LABELS[t as RecipeType] ?? t
}

function recipeQueryLink(recipe: Recipe): string {
  const q = new URLSearchParams()
  q.set('item_id', recipe.result_item_id)
  if (recipe.game_version) q.set('game_version', recipe.game_version)
  return `/recipes?${q.toString()}`
}

function externalUrl(url: string | null): string {
  return url && /^https?:\/\//i.test(url) ? url : '#'
}
</script>

<template>
  <PageContainer maxWidth="max-w-5xl">
    <Breadcrumbs :items="[{ label: '物品 ID 查询', to: '/items' }, { label: detail?.name_zh ?? itemId }]" />

    <LoadingSkeleton v-if="loading" :count="3" />
    <ErrorState v-else-if="error" :message="error" retryable @retry="loadDetail" />
    <EmptyState
      v-else-if="!detail"
      title="未收录该物品"
      description="本站物品库仅包含已核验收录的物品，未收录内容不会伪造查询结果。"
      action-label="返回物品查询"
      @action="router.push('/items')"
    />

    <template v-else>
      <!-- 头部信息 -->
      <header class="mb-6">
        <div class="mb-2 flex flex-wrap items-center gap-2">
          <span class="rounded border border-border-light bg-ink-700 px-2 py-0.5 text-xs text-text-muted">
            {{ itemCategoryLabel(detail.item_category) }}
          </span>
          <span class="rounded border border-border-light bg-ink-700 px-1.5 py-0.5 text-xs text-text-muted">
            {{ detail.game_edition === 'bedrock' ? '基岩版' : 'Java 版' }}
          </span>
        </div>
        <h1 class="mb-1 text-2xl font-bold text-text-primary md:text-3xl">{{ detail.name_zh }}</h1>
        <p v-if="detail.name_en" class="mb-3 text-text-secondary">{{ detail.name_en }}</p>

        <div class="flex flex-wrap items-center gap-3">
          <code class="rounded bg-ink-800 px-2 py-1 font-mono text-sm text-grass-300">{{ detail.item_id }}</code>
          <CopyButton :text="detail.item_id" label="复制 ID" />
          <CopyButton :text="detail.obtain_command" label="复制 /give" />
          <FavoriteButton
            kind="item"
            :slug="detail.item_id"
            :label="detail.name_zh"
            :url="`/items/${detail.item_id}`"
          />
        </div>

        <p v-if="detail.aliases?.length" class="mt-2 text-xs text-text-muted">
          别名：{{ detail.aliases.join('、') }}
        </p>
      </header>

      <!-- 基础属性 -->
      <section class="card mb-6 p-5">
        <h2 class="mb-3 font-semibold text-text-primary">基础属性</h2>
        <dl class="grid grid-cols-2 gap-x-4 gap-y-3 text-sm sm:grid-cols-3">
          <div>
            <dt class="text-xs text-text-muted">资料类别</dt>
            <dd class="text-text-primary">{{ itemCategoryLabel(detail.item_category) }}</dd>
          </div>
          <div>
            <dt class="text-xs text-text-muted">最大堆叠</dt>
            <dd class="text-text-primary">{{ detail.stack_size ?? '—' }}</dd>
          </div>
          <div v-if="showDurability">
            <dt class="text-xs text-text-muted">耐久度</dt>
            <dd class="text-text-primary">{{ detail.durability ?? '—' }}</dd>
          </div>
          <div>
            <dt class="text-xs text-text-muted">数据版本</dt>
            <dd class="text-text-primary">{{ detail.data_version || '—' }}</dd>
          </div>
          <div>
            <dt class="text-xs text-text-muted">最后校验</dt>
            <dd class="text-text-primary">{{ detail.last_verified_at ? formatDateTime(detail.last_verified_at) : '未经校验' }}</dd>
          </div>
        </dl>

        <div class="mt-4 flex flex-wrap items-center gap-2">
          <span class="text-xs text-text-muted">适用版本：</span>
          <template v-if="detail.supported_versions?.length">
            <VersionBadge v-for="v in detail.supported_versions" :key="v" :version="v" />
          </template>
          <span v-else class="text-xs text-text-muted">未标明</span>
        </div>
      </section>

      <!-- 获取方式 -->
      <section v-if="detail.obtain_method" class="card mb-6 p-5">
        <h2 class="mb-3 font-semibold text-text-primary">获取方式</h2>
        <p class="whitespace-pre-line text-sm text-text-secondary">{{ detail.obtain_method }}</p>
        <div class="mt-3 rounded-md border border-border-dark bg-ink-900 p-3">
          <div class="mb-1 text-xs text-text-muted">获取指令示例</div>
          <code class="font-mono text-sm text-grass-300">{{ detail.obtain_command }}</code>
        </div>
      </section>

      <!-- 用途 -->
      <section v-if="detail.usage" class="card mb-6 p-5">
        <h2 class="mb-3 font-semibold text-text-primary">用途</h2>
        <p class="whitespace-pre-line text-sm text-text-secondary">{{ detail.usage }}</p>
      </section>

      <!-- 描述 -->
      <section v-if="detail.description" class="card mb-6 p-5">
        <h2 class="mb-3 font-semibold text-text-primary">简介</h2>
        <p class="whitespace-pre-line text-sm text-text-secondary">{{ detail.description }}</p>
      </section>

      <!-- 关联配方（数据驱动） -->
      <section class="card mb-6 p-5">
        <h2 class="mb-3 font-semibold text-text-primary">关联配方</h2>

        <template v-if="hasRecipes">
          <div v-if="produced.length" class="mb-4">
            <h3 class="mb-2 text-sm font-medium text-text-primary">产出该物品的配方</h3>
            <ul class="space-y-2">
              <li
                v-for="recipe in produced"
                :key="recipe.id"
                class="rounded-md border border-border-dark bg-ink-800 px-3 py-2"
              >
                <div class="flex items-center justify-between gap-3 text-sm">
                  <span class="font-mono text-grass-300">
                    {{ shortItemId(recipe.result_item_id) }}{{ recipe.result_count > 1 ? ` ×${recipe.result_count}` : '' }}
                  </span>
                  <RouterLink
                    :to="recipeQueryLink(recipe)"
                    class="shrink-0 text-xs text-text-muted transition hover:text-grass-300"
                  >
                    查看配方 · {{ recipeTypeLabel(recipe.recipe_type) }} · {{ recipe.game_version || '—' }}
                  </RouterLink>
                </div>
                <div v-if="recipe.ingredients?.length" class="mt-2 flex flex-wrap items-center gap-1.5">
                  <span class="text-[10px] text-text-muted">材料：</span>
                  <template v-for="ing in ingredientLinks(recipe)" :key="ing.itemId">
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
              </li>
            </ul>
          </div>

          <div v-if="usedAs.length">
            <h3 class="mb-2 text-sm font-medium text-text-primary">作为材料的配方</h3>
            <ul class="space-y-2">
              <li
                v-for="recipe in usedAs"
                :key="recipe.id"
                class="rounded-md border border-border-dark bg-ink-800 px-3 py-2"
              >
                <div class="flex items-center justify-between gap-3 text-sm">
                  <!-- 产出物品与当前物品不同：已收录则给出物品详情页链接 -->
                  <RouterLink
                    v-if="itemLink(recipe.result_item_id).url"
                    :to="itemLink(recipe.result_item_id).url as string"
                    class="font-mono text-grass-300 hover:underline"
                  >
                    {{ itemLink(recipe.result_item_id).label }}
                  </RouterLink>
                  <span
                    v-else
                    class="font-mono text-text-secondary"
                    :title="`${recipe.result_item_id}（未收录物品资料）`"
                  >
                    {{ shortItemId(recipe.result_item_id) }}
                  </span>
                  <RouterLink
                    :to="recipeQueryLink(recipe)"
                    class="shrink-0 text-xs text-text-muted transition hover:text-grass-300"
                  >
                    查看配方 · {{ recipeTypeLabel(recipe.recipe_type) }} · {{ recipe.game_version || '—' }}
                  </RouterLink>
                </div>
                <div v-if="recipe.ingredients?.length" class="mt-2 flex flex-wrap items-center gap-1.5">
                  <span class="text-[10px] text-text-muted">材料：</span>
                  <template v-for="ing in ingredientLinks(recipe)" :key="ing.itemId">
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
              </li>
            </ul>
          </div>
        </template>
        <p v-else class="text-sm text-text-muted">暂未收录与该物品相关的配方。</p>
      </section>

      <!-- 跨模块检索入口 -->
      <section v-if="searchLinks" class="card mb-6 p-5">
        <h2 class="mb-1 font-semibold text-text-primary">在其它模块中查找</h2>
        <p class="mb-3 text-xs text-text-muted">
          以下为按名称发起的检索入口；本站不对物品与指令/百科/资源做无数据依据的关联。
        </p>
        <div class="flex flex-wrap gap-2">
          <RouterLink :to="searchLinks.command" class="rounded-md border border-border-light bg-ink-700 px-3 py-1.5 text-sm text-text-secondary transition hover:text-grass-300">
            检索相关指令
          </RouterLink>
          <RouterLink :to="searchLinks.wiki" class="rounded-md border border-border-light bg-ink-700 px-3 py-1.5 text-sm text-text-secondary transition hover:text-grass-300">
            检索百科条目
          </RouterLink>
          <RouterLink :to="searchLinks.resource" class="rounded-md border border-border-light bg-ink-700 px-3 py-1.5 text-sm text-text-secondary transition hover:text-grass-300">
            检索相关资源
          </RouterLink>
        </div>
      </section>

      <!-- 来源 -->
      <div class="flex flex-wrap items-center gap-3 rounded-card border border-dashed border-border-light p-4 text-xs text-text-muted">
        <span>数据来源：</span>
        <a
          v-if="detail.source_url"
          :href="externalUrl(detail.source_url)"
          target="_blank"
          rel="noopener noreferrer nofollow"
          class="text-grass-400 hover:underline"
        >
          {{ detail.source_url }}
        </a>
        <span v-else>未标注来源</span>
      </div>
    </template>
  </PageContainer>
</template>
