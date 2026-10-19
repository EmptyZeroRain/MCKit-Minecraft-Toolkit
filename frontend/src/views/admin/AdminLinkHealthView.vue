<script setup lang="ts">
// 资源链接健康与人工确认（TODO §6）
// 后台复查：受限并发检查资源来源链接；失效链接进入待确认列表，由人工确认失效 / 可用 / 暂不处理。
// 仅 admin 角色可见（侧边栏过滤 + 后端 403 兜底）。
import { computed, onMounted, reactive, ref } from 'vue'
import ListShell from '@/components/admin/list-shell.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useUiStore } from '@/stores/ui'
import { confirmLinkHealth, listLinkHealth, runLinkHealthCheck } from '@/api/admin-link-health'
import { ApiError } from '@/types/api'
import {
  LINK_CHECK_STATUS_LABELS,
  REVIEW_ACTION_LABELS,
  REVIEW_STATE_LABELS,
  statusClass,
  type LinkCheckStatus,
  type LinkHealthItem,
  type LinkHealthRunSummary,
  type ReviewAction,
  type ReviewState,
} from '@/types/link-health'
import { formatDateTime, timeAgo } from '@/utils/format'

const ui = useUiStore()

const filters = reactive<{
  reviewState: ReviewState | ''
  status: LinkCheckStatus | ''
  failedOnly: boolean
  page: number
  pageSize: number
}>({
  reviewState: '',
  status: '',
  failedOnly: true,
  page: 1,
  pageSize: 20,
})

const items = ref<LinkHealthItem[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')

const running = ref(false)
const summary = ref<LinkHealthRunSummary | null>(null)
// 每条记录的备注草稿（确认时随动作提交）
const notes = reactive<Record<string, string>>({})
const confirmingId = ref('')

const reviewOptions: Array<{ value: ReviewState | ''; label: string }> = [
  { value: '', label: '全部复核状态' },
  { value: 'pending', label: REVIEW_STATE_LABELS.pending },
  { value: 'confirmed_broken', label: REVIEW_STATE_LABELS.confirmed_broken },
  { value: 'confirmed_ok', label: REVIEW_STATE_LABELS.confirmed_ok },
  { value: 'ignored', label: REVIEW_STATE_LABELS.ignored },
]

const statusOptions: Array<{ value: LinkCheckStatus | ''; label: string }> = [
  { value: '', label: '全部链接状态' },
  { value: 'broken', label: LINK_CHECK_STATUS_LABELS.broken },
  { value: 'blocked', label: LINK_CHECK_STATUS_LABELS.blocked },
  { value: 'error', label: LINK_CHECK_STATUS_LABELS.error },
  { value: 'ok', label: LINK_CHECK_STATUS_LABELS.ok },
  { value: 'redirected', label: LINK_CHECK_STATUS_LABELS.redirected },
]

const hasFailures = computed(() => items.value.some((i) => i.status !== 'ok' && i.status !== 'redirected'))

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await listLinkHealth({
      review_state: filters.reviewState || undefined,
      status: filters.status || undefined,
      failed_only: filters.failedOnly,
      page: filters.page,
      page_size: filters.pageSize,
    })
    items.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function run() {
  running.value = true
  summary.value = null
  try {
    const res = await runLinkHealthCheck({ only_pending: true })
    summary.value = res
    ui.success(`复查完成：共检查 ${res.checked} 条，失效 ${res.broken}、被拦截 ${res.blocked}、错误 ${res.error}`)
    filters.page = 1
    await load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '复查失败')
  } finally {
    running.value = false
  }
}

async function confirm(item: LinkHealthItem, action: ReviewAction) {
  confirmingId.value = item.id
  try {
    const updated = await confirmLinkHealth(item.id, action, notes[item.id] ?? '')
    const idx = items.value.findIndex((i) => i.id === item.id)
    if (idx >= 0) items.value[idx] = { ...items.value[idx], ...updated }
    ui.success(`已${REVIEW_ACTION_LABELS[action]}：${item.resource_name || item.url}`)
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '确认失败')
  } finally {
    confirmingId.value = ''
  }
}

function onFilter() {
  filters.page = 1
  load()
}

function onPageChange(p: number) {
  filters.page = p
  load()
}

onMounted(load)
</script>

<template>
  <div class="space-y-6">
    <header>
      <h1 class="text-xl font-bold text-text-primary md:text-2xl">链接健康</h1>
      <p class="mt-1 text-sm text-text-secondary">
        后台复查资源来源链接的可达性；自动检查发现的失效链接进入「待确认」列表，由人工确认失效 / 可用 / 暂不处理。
        检查带超时、重定向次数与并发上限，且对出网频率限流。
      </p>
    </header>

    <div class="card p-4">
      <div class="flex flex-wrap items-end gap-3">
        <button
          type="button"
          class="rounded-md border border-grass-500 bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500 disabled:cursor-not-allowed disabled:opacity-60"
          :disabled="running"
          @click="run"
        >
          {{ running ? '复查中…' : '开始后台复查' }}
        </button>
        <label class="flex items-center gap-2 text-sm text-text-secondary">
          <input v-model="filters.failedOnly" type="checkbox" @change="onFilter" />
          只看失败项
        </label>
        <select v-model="filters.reviewState" class="rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary" @change="onFilter">
          <option v-for="o in reviewOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
        </select>
        <select v-model="filters.status" class="rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary" @change="onFilter">
          <option v-for="o in statusOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
        </select>
      </div>

      <p v-if="summary" class="mt-3 text-xs text-text-muted">
        本次检查 {{ summary.checked }} 条 · 正常 {{ summary.ok }} · 重定向 {{ summary.redirected }} · 失效
        {{ summary.broken }} · 被拦截 {{ summary.blocked }} · 错误 {{ summary.error }} · 并发上限
        {{ summary.concurrency }}
      </p>
    </div>

    <ListShell title="复核列表" :loading="loading" :error="error" @retry="load">
      <div class="card overflow-hidden">
        <div class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
              <tr>
                <th scope="col" class="px-4 py-3 font-medium">资源</th>
                <th scope="col" class="px-4 py-3 font-medium">链接状态</th>
                <th scope="col" class="px-4 py-3 font-medium">复核状态</th>
                <th scope="col" class="px-4 py-3 font-medium">最近检查</th>
                <th scope="col" class="px-4 py-3 font-medium">人工确认</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border-dark">
              <tr v-for="item in items" :key="item.id" class="align-top hover:bg-ink-800/60">
                <td class="px-4 py-3">
                  <div class="font-medium text-text-primary">{{ item.resource_name || '（未知资源）' }}</div>
                  <a
                    :href="item.url"
                    target="_blank"
                    rel="noopener noreferrer nofollow"
                    class="mt-0.5 block break-all font-mono text-xs text-grass-300 hover:underline"
                  >
                    {{ item.url }}
                  </a>
                  <div v-if="item.final_url && item.final_url !== item.url" class="mt-0.5 break-all text-xs text-text-muted">
                    最终地址：{{ item.final_url }}
                  </div>
                </td>
                <td class="px-4 py-3">
                  <span class="inline-flex rounded border px-1.5 py-0.5 text-xs" :class="statusClass(item.status)">
                    {{ LINK_CHECK_STATUS_LABELS[item.status] }}
                  </span>
                  <div class="mt-0.5 text-xs text-text-muted">HTTP {{ item.status_code || '—' }}</div>
                </td>
                <td class="px-4 py-3 text-text-secondary">
                  {{ REVIEW_STATE_LABELS[item.review_state] }}
                  <div v-if="item.reviewed_by" class="mt-0.5 text-xs text-text-muted">
                    由 {{ item.reviewed_by }} 于 {{ formatDateTime(item.reviewed_at) || '—' }}
                  </div>
                  <div v-if="item.review_note" class="mt-0.5 text-xs text-text-muted">{{ item.review_note }}</div>
                </td>
                <td class="px-4 py-3 text-xs text-text-muted" :title="formatDateTime(item.checked_at)">
                  {{ timeAgo(item.checked_at) }}
                  <div>来源：{{ item.trigger_source === 'manual' ? '人工触发' : '后台复查' }}</div>
                </td>
                <td class="px-4 py-3">
                  <input
                    v-model="notes[item.id]"
                    type="text"
                    placeholder="备注（可选）"
                    class="mb-2 w-full rounded-md border border-border-dark bg-ink-900 px-2 py-1 text-xs text-text-primary placeholder:text-text-muted"
                  />
                  <div class="flex flex-wrap gap-2">
                    <button
                      v-for="action in (['confirm_broken', 'confirm_ok', 'ignore'] as ReviewAction[])"
                      :key="action"
                      type="button"
                      class="rounded border border-border-light bg-ink-700 px-2 py-1 text-xs text-text-secondary transition hover:bg-ink-600 hover:text-text-primary disabled:opacity-50"
                      :disabled="confirmingId === item.id"
                      @click="confirm(item, action)"
                    >
                      {{ REVIEW_ACTION_LABELS[action] }}
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <p v-if="!items.length" class="px-4 py-8 text-center text-sm text-text-muted">
          {{ hasFailures ? '暂无记录' : '暂无失败项——当前筛选下所有链接均可达或已人工确认。' }}
        </p>
      </div>

      <Pagination
        class="mt-4"
        :page="filters.page"
        :page-size="filters.pageSize"
        :total="total"
        :total-pages="totalPages"
        @change="onPageChange"
      />
    </ListShell>
  </div>
</template>
