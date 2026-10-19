<script setup lang="ts">
// 物品 ID 对照表（移植自 CreeperCode 的 BlockIdPage）
// 展示 Minecraft 方块/物品的「中文名 / 命名空间 ID / 物品形式」对照，
// 按物品形式分类着色并提供关键词 + 分类筛选。
// 数据量约千余条且静态：一次全量加载，筛选在本地完成。
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import CopyButton from '@/components/common/CopyButton.vue'
import ShareButton from '@/components/common/ShareButton.vue'
import FilterPanel from '@/components/common/FilterPanel.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import { listBlockIds } from '@/api/block-ids'
import { ApiError } from '@/types/api'
import {
  BLOCK_ID_KINDS,
  BLOCK_ID_KIND_COLORS,
  BLOCK_ID_KIND_LABELS,
  type BlockIdEntry,
} from '@/types/block-id'
import { countBlockIds, filterBlockIds } from '@/utils/block-ids'
import { useRecentToolsStore } from '@/stores/recent-tools'
import { applyPageMeta } from '@/utils/seo'
import { changedOnly, queryString } from '@/utils/share'

applyPageMeta({
  title: '物品 ID 对照表',
  description:
    'Minecraft 方块与物品的命名空间 ID 对照表，按物品形式分类，支持中文名与 ID 关键词筛选，一键复制 ID。',
  canonicalPath: '/block-ids',
})

useRecentToolsStore().record('block-ids', '物品 ID 对照表', '查询类')

const route = useRoute()
const router = useRouter()

// TODO §14：筛选状态同步到 URL，使「某个关键词 / 分类的筛选结果」可直接分享与复现。
const keyword = ref(queryString(route.query.q))
const kind = ref(queryString(route.query.kind))

const entries = ref<BlockIdEntry[]>([])
const loading = ref(false)
const error = ref('')

const filtered = computed(() => filterBlockIds(entries.value, keyword.value, kind.value))

/** 将筛选状态写入 URL（仅保留非默认值）。 */
function syncUrl() {
  router.replace({
    path: '/block-ids',
    query: changedOnly({ q: keyword.value, kind: kind.value }, { q: '', kind: '' }),
  })
}

watch([keyword, kind], syncUrl)

/** 分类按钮文案带计数（对齐 CreeperCode 的筛选体验） */
const kindOptions = computed(() =>
  BLOCK_ID_KINDS.map((k) => ({
    value: k,
    label: `${BLOCK_ID_KIND_LABELS[k]}（${countBlockIds(entries.value, k)}）`,
  })),
)

function kindLabel(k: string): string {
  return BLOCK_ID_KIND_LABELS[k as keyof typeof BLOCK_ID_KIND_LABELS] ?? k
}

function kindColor(k: string): string {
  return BLOCK_ID_KIND_COLORS[k as keyof typeof BLOCK_ID_KIND_COLORS] ?? 'inherit'
}

async function loadEntries() {
  loading.value = true
  error.value = ''
  try {
    const res = await listBlockIds({})
    entries.value = res.entries
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败，请稍后重试'
    entries.value = []
  } finally {
    loading.value = false
  }
}

function clearSearch() {
  keyword.value = ''
}

function resetFilters() {
  keyword.value = ''
  kind.value = ''
}

onMounted(loadEntries)
</script>

<template>
  <PageContainer maxWidth="max-w-6xl">
    <Breadcrumbs :items="[{ label: '物品 ID 对照表' }]" />
    <div class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">物品 ID 对照表</h1>
        <p class="text-text-secondary">
          方块与物品的「中文名 / 命名空间 ID / 物品形式」对照，按物品形式分类着色，支持关键词与分类筛选。
        </p>
      </div>
      <ShareButton />
    </div>

    <!-- 搜索 -->
    <div class="relative mb-4 max-w-xl">
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
        v-model="keyword"
        type="search"
        aria-label="搜索 ID 或中文名"
        placeholder="搜索 ID 或中文名，例如：stone / 石头 / 苹果"
        class="h-11 w-full rounded-md border border-border-dark bg-ink-800 pl-9 pr-9 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
      />
      <button
        v-if="keyword"
        class="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted hover:text-text-primary"
        aria-label="清除"
        @click="clearSearch"
      >
        ✕
      </button>
    </div>

    <!-- 分类筛选 -->
    <div class="mb-6 space-y-2">
      <FilterPanel
        v-model="kind"
        title="物品形式分类"
        :options="kindOptions"
        all-label="全部"
      />
      <p class="text-xs text-text-muted">
        按物品形式着色：
        <span
          v-for="k in BLOCK_ID_KINDS"
          :key="k"
          class="mr-3 inline-flex items-center gap-1"
        >
          <span
            class="inline-block h-2.5 w-2.5 rounded-sm"
            :style="{ backgroundColor: kindColor(k) }"
            aria-hidden="true"
          />
          {{ kindLabel(k) }}
        </span>
      </p>
    </div>

    <LoadingSkeleton v-if="loading" :count="6" />
    <ErrorState v-else-if="error" :message="error" retryable @retry="loadEntries" />

    <template v-else>
      <div class="mb-3 text-sm text-text-muted">
        共 {{ filtered.length }} 条<template v-if="filtered.length !== entries.length">
          （全部 {{ entries.length }} 条）</template
        >
      </div>

      <div v-if="filtered.length" class="card overflow-hidden">
        <div class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
              <tr>
                <th scope="col" class="px-4 py-3 font-medium">名称</th>
                <th scope="col" class="px-4 py-3 font-medium">命名空间 ID</th>
                <th scope="col" class="hidden px-4 py-3 font-medium md:table-cell">物品形式</th>
                <th scope="col" class="px-4 py-3 text-right font-medium">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border-dark">
              <tr v-for="e in filtered" :key="e.kind + e.id" class="hover:bg-ink-800/60">
                <td class="px-4 py-3 text-text-primary">{{ e.name || '—' }}</td>
                <td class="px-4 py-3 font-mono text-grass-300">{{ e.id }}</td>
                <td
                  class="hidden px-4 py-3 md:table-cell"
                  :style="{ color: kindColor(e.kind) }"
                  :title="kindLabel(e.kind)"
                >
                  {{ e.form || kindLabel(e.kind) }}
                </td>
                <td class="px-4 py-3 text-right">
                  <CopyButton tool="block-ids" :text="e.id" label="复制 ID" />
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <EmptyState
        v-else
        title="没有匹配的条目"
        description="试试更换关键词，或切换到「全部」分类重新查看。"
        action-label="清除筛选"
        @action="resetFilters"
      />
    </template>
  </PageContainer>
</template>
