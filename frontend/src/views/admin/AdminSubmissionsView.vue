<script setup lang="ts">
// 资源投稿审核（用户提交结构化资源 → 通过则生成资源）。
//
// 权限：content.resources（审核通过会创建 resources 记录，与「资源管理」同权能）。
// 默认只看待审核项；通过默认生成草稿资源（需在资源管理中核验后发布），
// 也可直接「通过并发布」。审核人取自登录会话并写入审计日志。
import { computed, onMounted, reactive, ref } from 'vue'
import ListShell from '@/components/admin/list-shell.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useUiStore } from '@/stores/ui'
import { batchReviewSubmissions, listSubmissions, reviewSubmission } from '@/api/admin-submissions'
import { describeBatchResult, MAX_BATCH_REVIEW } from '@/types/batch'
import { ApiError } from '@/types/api'
import { RESOURCE_TYPES, RESOURCE_TYPE_LABELS, LOADER_LABELS } from '@/types/common'
import {
  SUBMISSION_BADGE_STATUS,
  SUBMISSION_STATUS_LABELS,
  type ResourceSubmission,
  type SubmissionStatus,
} from '@/types/submission'
import { formatDateTime, joinList } from '@/utils/format'

const ui = useUiStore()

const filters = reactive<{
  status: SubmissionStatus | ''
  resourceType: string
  q: string
  page: number
  pageSize: number
}>({
  status: 'pending',
  resourceType: '',
  q: '',
  page: 1,
  pageSize: 20,
})

const items = ref<ResourceSubmission[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')
/** 每条记录的审核备注草稿 */
const notes = reactive<Record<string, string>>({})
const reviewingId = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await listSubmissions({
      status: filters.status || undefined,
      resource_type: filters.resourceType || undefined,
      q: filters.q.trim() || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    })
    items.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
    // 记录状态变化后，已勾选项可能已不可再审核：只保留仍在待审核列表中的选择。
    const stillPending = new Set(
      res.items.filter((i: ResourceSubmission) => i.status === 'pending').map((i: ResourceSubmission) => i.id),
    )
    selected.value = new Set([...selected.value].filter((id) => stillPending.has(id)))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

/** status: approved（可选 publish 直接发布）/ rejected */
async function review(item: ResourceSubmission, status: 'approved' | 'rejected', publish = false) {
  reviewingId.value = item.id
  try {
    const result = await reviewSubmission({
      id: item.id,
      status,
      note: notes[item.id] ?? '',
      publish,
    })
    if (status === 'approved') {
      const resStatus = result.resource?.status
      ui.success(
        resStatus === 'published'
          ? `已通过并发布：${item.name}`
          : `已通过，已生成草稿资源：${item.name}（请在资源管理中核验后发布）`,
      )
    } else {
      ui.success(`已驳回：${item.name}`)
    }
    notes[item.id] = ''
    await load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '审核失败')
  } finally {
    reviewingId.value = ''
  }
}

// ===== 批量审核 =====
// 只允许勾选「待审核」的记录：已审核项勾选后提交必然失败，提前禁用减少噪音。
const selected = ref<Set<string>>(new Set())
const batchNote = ref('')
const batchBusy = ref(false)

const pendingItems = computed(() => items.value.filter((i) => i.status === 'pending'))
const allSelected = computed(
  () => pendingItems.value.length > 0 && pendingItems.value.every((i) => selected.value.has(i.id)),
)

function toggleOne(id: string) {
  const next = new Set(selected.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selected.value = next
}

function toggleAll() {
  selected.value = allSelected.value ? new Set() : new Set(pendingItems.value.map((i) => i.id))
}

async function batchReview(status: 'approved' | 'rejected', publish = false) {
  if (batchBusy.value) return
  const ids = [...selected.value]
  if (!ids.length) {
    ui.warning('请先勾选要审核的投稿')
    return
  }
  if (ids.length > MAX_BATCH_REVIEW) {
    ui.error(`单次最多 ${MAX_BATCH_REVIEW} 条，请分批提交`)
    return
  }
  if (
    status === 'rejected' &&
    !window.confirm(`将驳回 ${ids.length} 条投稿，确认继续？`)
  ) {
    return
  }
  batchBusy.value = true
  try {
    const result = await batchReviewSubmissions({
      ids,
      status,
      note: batchNote.value.trim(),
      publish,
    })
    const action = status === 'rejected' ? '批量驳回' : publish ? '批量通过并发布' : '批量通过'
    if (result.failed > 0) ui.warning(describeBatchResult(result, action))
    else ui.success(describeBatchResult(result, action))
    selected.value = new Set()
    batchNote.value = ''
    await load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '批量审核失败')
  } finally {
    batchBusy.value = false
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
      <h1 class="text-xl font-bold text-text-primary md:text-2xl">资源投稿审核</h1>
      <p class="mt-1 text-sm text-text-secondary">
        审核用户提交的结构化资源：默认展示待审核项。「通过」会按投稿字段生成一条草稿资源
        （在「资源管理」中核验后发布），也可选择「通过并发布」；驳回需说明原因。
        审核人取自登录会话，状态与备注完整留痕并写入审计日志。
      </p>
    </header>

    <div class="card p-4">
      <div class="flex flex-wrap items-end gap-3">
        <select
          v-model="filters.status"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="onFilter"
        >
          <option value="">全部审核状态</option>
          <option value="pending">{{ SUBMISSION_STATUS_LABELS.pending }}</option>
          <option value="approved">{{ SUBMISSION_STATUS_LABELS.approved }}</option>
          <option value="rejected">{{ SUBMISSION_STATUS_LABELS.rejected }}</option>
        </select>
        <select
          v-model="filters.resourceType"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="onFilter"
        >
          <option value="">全部资源类型</option>
          <option v-for="t in RESOURCE_TYPES" :key="t" :value="t">{{ RESOURCE_TYPE_LABELS[t] }}</option>
        </select>
        <input
          v-model="filters.q"
          type="search"
          placeholder="搜索名称 / 简介 / 提交者"
          class="h-9 w-56 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          @keyup.enter="onFilter"
        />
        <button
          type="button"
          class="h-9 rounded-md border border-border-light bg-ink-700 px-4 text-sm text-text-secondary transition hover:bg-ink-600"
          @click="onFilter"
        >
          筛选
        </button>
      </div>
    </div>

    <div
      v-if="pendingItems.length"
      class="card flex flex-wrap items-center gap-3 p-3 text-sm"
    >
      <label class="flex items-center gap-2 text-text-secondary">
        <input
          type="checkbox"
          :checked="allSelected"
          class="h-4 w-4 accent-grass-500"
          @change="toggleAll"
        />
        全选本页待审核（{{ pendingItems.length }}）
      </label>
      <input
        v-model="batchNote"
        type="text"
        placeholder="批量审核备注（可选，会写入每一条）"
        class="h-8 w-64 rounded-md border border-border-dark bg-ink-800 px-2 text-xs text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
      />
      <div class="flex flex-wrap gap-2">
        <button
          type="button"
          class="rounded border border-grass-500/50 bg-grass-500/10 px-3 py-1.5 text-xs text-grass-300 transition hover:bg-grass-500/20 disabled:opacity-40"
          :disabled="batchBusy || selected.size === 0"
          @click="batchReview('approved')"
        >
          批量通过（草稿）
        </button>
        <button
          type="button"
          class="rounded border border-grass-500/50 bg-grass-500/10 px-3 py-1.5 text-xs text-grass-300 transition hover:bg-grass-500/20 disabled:opacity-40"
          :disabled="batchBusy || selected.size === 0"
          @click="batchReview('approved', true)"
        >
          批量通过并发布
        </button>
        <button
          type="button"
          class="rounded border border-danger-mc/50 bg-danger-mc/10 px-3 py-1.5 text-xs text-danger-mc transition hover:bg-danger-mc/20 disabled:opacity-40"
          :disabled="batchBusy || selected.size === 0"
          @click="batchReview('rejected')"
        >
          批量驳回
        </button>
      </div>
      <span class="text-xs text-text-muted">
        已选 {{ selected.size }} 条{{ batchBusy ? ' · 处理中…' : '' }}
      </span>
    </div>

    <ListShell title="投稿列表" :loading="loading" :error="error" @retry="load">
      <div class="card overflow-hidden">
        <div class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
              <tr>
                <th scope="col" class="w-10 px-4 py-3 font-medium">
                  <span class="sr-only">选择</span>
                </th>
                <th scope="col" class="px-4 py-3 font-medium">资源</th>
                <th scope="col" class="px-4 py-3 font-medium">类型 / 加载器</th>
                <th scope="col" class="px-4 py-3 font-medium">提交者</th>
                <th scope="col" class="px-4 py-3 font-medium">来源 / 许可</th>
                <th scope="col" class="px-4 py-3 font-medium">审核状态</th>
                <th scope="col" class="px-4 py-3 font-medium">审核操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border-dark">
              <tr v-for="item in items" :key="item.id" class="align-top hover:bg-ink-800/60">
                <td class="px-4 py-3">
                  <input
                    v-if="item.status === 'pending'"
                    type="checkbox"
                    :checked="selected.has(item.id)"
                    class="mt-1 h-4 w-4 accent-grass-500"
                    :aria-label="`选择投稿 ${item.name}`"
                    @change="toggleOne(item.id)"
                  />
                </td>
                <td class="px-4 py-3">
                  <div class="font-medium text-text-primary">{{ item.name }}</div>
                  <div class="mt-0.5 max-w-md text-xs text-text-secondary">{{ item.summary }}</div>
                  <div class="mt-0.5 text-xs text-text-muted">
                    适用版本：{{ joinList(item.supported_versions) || '未标注' }}
                  </div>
                </td>
                <td class="px-4 py-3 text-text-secondary">
                  {{ RESOURCE_TYPE_LABELS[item.resource_type] }}
                  <div class="mt-0.5 text-xs text-text-muted">
                    {{ joinList(item.loader_types.map((l) => LOADER_LABELS[l])) || '未标注加载器' }}
                  </div>
                </td>
                <td class="px-4 py-3 text-xs text-text-secondary">
                  {{ item.submitter_name || '—' }}
                  <div class="mt-0.5 text-text-muted">{{ formatDateTime(item.created_at) }}</div>
                </td>
                <td class="px-4 py-3">
                  <a
                    :href="item.source_url"
                    target="_blank"
                    rel="noopener noreferrer nofollow"
                    class="break-all font-mono text-xs text-grass-300 hover:underline"
                  >
                    {{ item.source_url }}
                  </a>
                  <div class="mt-0.5 text-xs text-text-muted">
                    {{ item.source_platform || '未标注平台' }} · 许可：{{ item.license || '未标注' }}
                  </div>
                  <div v-if="item.author_name" class="mt-0.5 text-xs text-text-muted">
                    作者：{{ item.author_name }}
                  </div>
                  <div v-if="item.note" class="mt-1 max-w-sm rounded border border-border-dark bg-ink-900/60 px-2 py-1 text-xs text-text-secondary">
                    投稿说明：{{ item.note }}
                  </div>
                </td>
                <td class="px-4 py-3">
                  <StatusBadge
                    :status="SUBMISSION_BADGE_STATUS[item.status]"
                    :label="SUBMISSION_STATUS_LABELS[item.status]"
                  />
                  <div v-if="item.reviewed_at" class="mt-1 text-xs text-text-muted">
                    最近审核：{{ item.reviewer || '（未记录审核人）' }} · {{ formatDateTime(item.reviewed_at) }}
                    <template v-if="item.review_note"> · {{ item.review_note }}</template>
                  </div>
                </td>
                <td class="px-4 py-3">
                  <template v-if="item.status === 'pending'">
                    <input
                      v-model="notes[item.id]"
                      type="text"
                      placeholder="审核备注（可选）"
                      class="mb-2 w-full rounded-md border border-border-dark bg-ink-900 px-2 py-1 text-xs text-text-primary placeholder:text-text-muted"
                    />
                    <div class="flex flex-wrap gap-2">
                      <button
                        type="button"
                        class="rounded border border-grass-500/50 bg-grass-500/10 px-2 py-1 text-xs text-grass-300 transition hover:bg-grass-500/20 disabled:opacity-50"
                        :disabled="reviewingId === item.id"
                        @click="review(item, 'approved')"
                      >
                        通过（生成草稿）
                      </button>
                      <button
                        type="button"
                        class="rounded border border-grass-500/50 bg-grass-500/10 px-2 py-1 text-xs text-grass-300 transition hover:bg-grass-500/20 disabled:opacity-50"
                        :disabled="reviewingId === item.id"
                        @click="review(item, 'approved', true)"
                      >
                        通过并发布
                      </button>
                      <button
                        type="button"
                        class="rounded border border-danger-mc/50 bg-danger-mc/10 px-2 py-1 text-xs text-danger-mc transition hover:bg-danger-mc/20 disabled:opacity-50"
                        :disabled="reviewingId === item.id"
                        @click="review(item, 'rejected')"
                      >
                        驳回
                      </button>
                    </div>
                  </template>
                  <span v-else class="text-xs text-text-muted">已审核</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-if="!items.length" class="px-4 py-8 text-center text-sm text-text-muted">
          暂无投稿——当前筛选下没有资源投稿。
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
