<script setup lang="ts">
// 搜索词统计（TODO §5 搜索体验）
// 仅 admin 角色可见（侧边栏过滤 + 后端 403 兜底）。
// 数据为匿名化聚合结果：只含归一化关键词与计数，不含 IP / 会话 / 用户标识。
import { computed, onMounted, reactive, ref, watch } from 'vue'
import ListShell from '@/components/admin/list-shell.vue'
import AdminTable from '@/components/admin/AdminTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import { listSearchTerms } from '@/api/admin-search-terms'
import type { AdminTableColumn } from '@/types/admin'
import { ApiError } from '@/types/api'
import type { SearchTermKind, SearchTermStat } from '@/types/search-term'
import { isHighNoResult, noResultRatio } from '@/utils/search-term'
import { timeAgo } from '@/utils/format'

const filters = reactive<{ kind: SearchTermKind; q: string; page: number; pageSize: number }>({
  kind: 'hot',
  q: '',
  page: 1,
  pageSize: 20,
})

const terms = ref<SearchTermStat[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')

const kindOptions: Array<{ label: string; value: SearchTermKind; hint: string }> = [
  { label: '热门搜索', value: 'hot', hint: '按被搜索总次数排序，反映真实关注点。' },
  { label: '高频无结果词', value: 'no_result', hint: '搜索后没有结果的关键词，代表资料缺口。' },
  { label: '查询失败词', value: 'failed', hint: '查询本身失败（超时 / 服务错误）的关键词，需排查链路问题。' },
]

const currentHint = computed(() => kindOptions.find((o) => o.value === filters.kind)?.hint ?? '')

const columns: AdminTableColumn[] = [
  { key: 'term', label: '关键词' },
  { key: 'hits', label: '搜索次数', width: '110px' },
  { key: 'no_result_hits', label: '无结果次数', width: '110px' },
  { key: 'failed_hits', label: '失败次数', width: '100px' },
  { key: 'ratio', label: '无结果占比', width: '110px' },
  { key: 'last_result_count', label: '最近一次结果数', width: '140px' },
  { key: 'last_seen_at', label: '最近搜索', width: '120px' },
]

/** 空态文案随当前统计维度变化。 */
const emptyText = computed(() =>
  filters.kind === 'no_result'
    ? '暂无无结果搜索记录。'
    : filters.kind === 'failed'
      ? '暂无查询失败记录。'
      : '暂无搜索记录。',
)

async function loadTerms() {
  loading.value = true
  error.value = ''
  try {
    const res = await listSearchTerms({
      kind: filters.kind,
      q: filters.q.trim() || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    })
    terms.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

function onKindChange(kind: SearchTermKind) {
  if (filters.kind === kind) return
  filters.kind = kind
  filters.page = 1
  loadTerms()
}

function onPageChange(p: number) {
  filters.page = p
  loadTerms()
}

watch(
  () => filters.q,
  () => {
    filters.page = 1
    loadTerms()
  },
)

onMounted(loadTerms)
</script>

<template>
  <div class="space-y-5">
    <div class="flex items-center justify-between gap-4">
      <div>
        <h1 class="text-lg font-semibold text-text-primary">搜索词统计</h1>
        <p class="mt-1 text-sm text-text-muted">
          仅记录归一化关键词与次数，不含 IP、会话或用户标识；用于发现热门需求与资料缺口。
        </p>
      </div>
      <button
        type="button"
        class="shrink-0 rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-primary hover:bg-ink-600 disabled:opacity-50"
        :disabled="loading"
        @click="loadTerms"
      >
        刷新
      </button>
    </div>

    <ListShell title="搜索词" :loading="loading" :error="error" @retry="loadTerms">
      <div class="mb-4 flex flex-wrap items-center gap-3">
        <div class="flex flex-wrap gap-2">
          <button
            v-for="opt in kindOptions"
            :key="opt.value"
            type="button"
            class="rounded-full border px-3 py-1 text-xs transition"
            :class="
              filters.kind === opt.value
                ? 'border-grass-500 bg-grass-500/15 text-grass-300'
                : 'border-border-dark bg-ink-700 text-text-secondary hover:bg-ink-600'
            "
            @click="onKindChange(opt.value)"
          >
            {{ opt.label }}
          </button>
        </div>
        <input
          v-model="filters.q"
          type="search"
          maxlength="64"
          placeholder="过滤关键词…"
          aria-label="过滤关键词"
          class="h-9 w-full max-w-xs rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
        />
      </div>

      <p class="mb-3 text-xs text-text-muted">{{ currentHint }}</p>

      <AdminTable
        :columns="columns"
        :rows="terms"
        row-key="id"
        min-width="720px"
        :empty-text="emptyText"
      >
        <template #cell-term="{ row }">
          <span class="text-text-primary">{{ row.term }}</span>
          <span
            v-if="filters.kind === 'no_result' && isHighNoResult(row)"
            class="ml-2 rounded bg-danger-mc/15 px-1.5 py-0.5 text-xs text-danger-mc"
          >
            高频缺口
          </span>
        </template>
        <template #cell-no_result_hits="{ row }">
          <span :class="row.no_result_hits ? 'text-amber-300' : 'text-text-muted'">
            {{ row.no_result_hits }}
          </span>
        </template>
        <template #cell-failed_hits="{ row }">
          <span :class="row.failed_hits ? 'text-danger-mc' : 'text-text-muted'">
            {{ row.failed_hits }}
          </span>
        </template>
        <template #cell-ratio="{ row }">
          <span class="text-text-secondary">{{ noResultRatio(row) }}%</span>
        </template>
        <template #cell-last_seen_at="{ row }">
          <span class="text-xs text-text-muted">{{ timeAgo(row.last_seen_at) }}</span>
        </template>
      </AdminTable>
    </ListShell>

    <Pagination
      :page="filters.page"
      :page-size="filters.pageSize"
      :total="total"
      :total-pages="totalPages"
      @change="onPageChange"
    />
  </div>
</template>
