<script setup lang="ts">
// 物品 ID 查询（需求 5.4）
import { onMounted, reactive, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import ShareButton from '@/components/common/ShareButton.vue'
import CopyButton from '@/components/common/CopyButton.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import Pagination from '@/components/common/Pagination.vue'
import { listItems } from '@/api/public'
import type { Item } from '@/types/item'
import { ApiError } from '@/types/api'
import { useRecentToolsStore } from '@/stores/recent-tools'
import { applyPageMeta } from '@/utils/seo'

applyPageMeta({
  title: '物品 ID 查询',
  description: '按中文名、英文名、标准 ID、别名搜索 Minecraft 物品，一键复制 namespaced ID。',
  canonicalPath: '/items',
})

// 记录最近使用
useRecentToolsStore().record('items', '物品 ID 查询', '查询类')

const route = useRoute()
const router = useRouter()

const query = reactive({
  q: (route.query.q as string) || '',
  page: Number(route.query.page) || 1,
  pageSize: 15,
})

const items = ref<Item[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')

async function loadItems() {
  loading.value = true
  error.value = ''
  try {
    const res = await listItems({ q: query.q || undefined, page: query.page, page_size: query.pageSize })
    items.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / query.pageSize))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

function syncUrl() {
  const params: Record<string, string> = {}
  if (query.q) params.q = query.q
  if (query.page > 1) params.page = String(query.page)
  router.replace({ path: '/items', query: params })
}

watch(
  () => query.q,
  () => {
    query.page = 1
    syncUrl()
    loadItems()
  },
)

function onPageChange(p: number) {
  query.page = p
  syncUrl()
  loadItems()
}

function clearSearch() {
  query.q = ''
}

onMounted(loadItems)
</script>

<template>
  <PageContainer maxWidth="max-w-6xl">
    <Breadcrumbs :items="[{ label: '物品 ID 查询' }]" />
    <div class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">物品 ID 查询</h1>
        <p class="text-text-secondary">输入中文名、英文名、标准 ID 或别名查询物品。</p>

      </div>
      <ShareButton />
    </div>

    <!-- 搜索 -->
    <div class="mb-6 relative max-w-xl">
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
        v-model="query.q"
        type="search"
        aria-label="搜索物品"
        placeholder="例如：石头 / stone / minecraft:stone"
        class="h-11 w-full rounded-md border border-border-dark bg-ink-800 pl-9 pr-9 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
      />
      <button
        v-if="query.q"
        class="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted hover:text-text-primary"
        aria-label="清除"
        @click="clearSearch"
      >
        ✕
      </button>
    </div>

    <LoadingSkeleton v-if="loading" :count="5" />
    <ErrorState v-else-if="error" :message="error" retryable @retry="loadItems" />

    <div v-else-if="items.length" class="card overflow-hidden">
      <div class="overflow-x-auto">
        <table class="w-full text-left text-sm">
          <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
            <tr>
              <th scope="col" class="px-4 py-3 font-medium">物品 ID</th>
              <th scope="col" class="px-4 py-3 font-medium">中文名</th>
              <th scope="col" class="hidden px-4 py-3 font-medium md:table-cell">英文名</th>
              <th scope="col" class="hidden px-4 py-3 font-medium lg:table-cell">说明</th>
              <th scope="col" class="px-4 py-3 text-right font-medium">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border-dark">
            <tr v-for="item in items" :key="item.id" class="hover:bg-ink-800/60">
              <td class="px-4 py-3 font-mono">
                <RouterLink
                  :to="{ name: 'item-detail', params: { itemId: item.item_id } }"
                  class="text-grass-300 hover:underline"
                >
                  {{ item.item_id }}
                </RouterLink>
              </td>
              <td class="px-4 py-3 text-text-primary">
                <RouterLink
                  :to="{ name: 'item-detail', params: { itemId: item.item_id } }"
                  class="hover:text-grass-300"
                >
                  {{ item.name_zh }}
                </RouterLink>
              </td>
              <td class="hidden px-4 py-3 text-text-secondary md:table-cell">{{ item.name_en || '—' }}</td>
              <td class="hidden max-w-xs truncate px-4 py-3 text-text-muted lg:table-cell">
                {{ item.description || '—' }}
              </td>
              <td class="px-4 py-3 text-right">
                <div class="flex items-center justify-end gap-3">
                  <RouterLink
                    :to="{ name: 'item-detail', params: { itemId: item.item_id } }"
                    class="text-text-secondary transition hover:text-grass-300"
                  >
                    详情
                  </RouterLink>
                  <CopyButton tool="items" :text="item.item_id" label="复制 ID" />
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <EmptyState
      v-else
      title="未收录该物品"
      description="本站物品库仅包含已核验收录的物品；未收录内容不会伪造查询结果。试试其他关键词？"
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