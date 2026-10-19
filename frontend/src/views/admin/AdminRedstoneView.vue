<script setup lang="ts">
// 红石机器设计内容审核（TODO §15，admin 专属）
// 后台按审核状态 / 用途筛选机器设计；默认只看待审核，逐条「通过 / 驳回」并留备注。
// 仅 admin 角色可见（侧边栏过滤 + 后端 403 兜底）。
import { computed, onMounted, reactive, ref } from 'vue'
import ListShell from '@/components/admin/list-shell.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useUiStore } from '@/stores/ui'
import {
  batchReviewRedstoneDesigns,
  listRedstoneDesignsAdmin,
  reviewRedstoneDesign,
} from '@/api/admin-redstone'
import { describeBatchResult, MAX_BATCH_REVIEW } from '@/types/batch'
import { ApiError } from '@/types/api'
import {
  REDSTONE_DESIGN_PURPOSES,
  REDSTONE_DESIGN_PURPOSE_LABELS,
  REDSTONE_DIFFICULTY_LABELS,
  isExternalUrl,
  type RedstoneDesign,
  type RedstoneDesignPurpose,
  type RedstoneReviewStatus,
} from '@/types/redstone'
import { formatDateTime, joinList } from '@/utils/format'

const ui = useUiStore()

// 审核状态中文标签：审核语义与公开侧「已发布」不同，这里统一为「已通过」。
const REVIEW_STATUS_LABELS: Record<RedstoneReviewStatus, string> = {
  pending: '待审核',
  published: '已通过',
  rejected: '已驳回',
}

// 复用 StatusBadge 调色板：文字由 label 覆盖为审核语义，仅借用颜色。
const REVIEW_BADGE_STATUS: Record<RedstoneReviewStatus, string> = {
  pending: 'processing',
  published: 'published',
  rejected: 'removed',
}

const filters = reactive<{
  reviewStatus: RedstoneReviewStatus | ''
  purpose: RedstoneDesignPurpose | ''
  page: number
  pageSize: number
}>({
  reviewStatus: 'pending',
  purpose: '',
  page: 1,
  pageSize: 20,
})

const items = ref<RedstoneDesign[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')
/** 每条设计的审核备注草稿（审核时随状态一并提交） */
const notes = reactive<Record<string, string>>({})
const reviewingId = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await listRedstoneDesignsAdmin({
      review_status: filters.reviewStatus || undefined,
      purpose: filters.purpose || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    })
    items.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
    // 刷新后只保留仍在待审核列表中的选择。
    const stillPending = new Set(
      res.items.filter((i) => i.review_status === 'pending').map((i) => i.id),
    )
    selected.value = new Set([...selected.value].filter((id) => stillPending.has(id)))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function review(item: RedstoneDesign, status: RedstoneReviewStatus) {
  reviewingId.value = item.id
  try {
    await reviewRedstoneDesign(item.id, status, notes[item.id] ?? '')
    ui.success(`已${REVIEW_STATUS_LABELS[status]}：${item.name_zh || item.slug}`)
    notes[item.id] = ''
    await load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '审核失败')
  } finally {
    reviewingId.value = ''
  }
}

// ===== 批量审核 =====
// 只允许勾选待审核记录：已审核项提交必然失败，提前禁用减少噪音。
const selected = ref<Set<string>>(new Set())
const batchNote = ref('')
const batchBusy = ref(false)

const pendingRows = computed(() =>
  items.value
    .filter((i) => i.review_status === 'pending')
    .map((i) => ({ id: i.id, name: i.name_zh || i.slug })),
)
const allSelected = computed(
  () => pendingRows.value.length > 0 && pendingRows.value.every((r) => selected.value.has(r.id)),
)

function toggleOne(id: string) {
  const next = new Set(selected.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selected.value = next
}

function toggleAll() {
  selected.value = allSelected.value ? new Set() : new Set(pendingRows.value.map((r) => r.id))
}

async function batchReview(status: RedstoneReviewStatus) {
  if (batchBusy.value) return
  const ids = [...selected.value]
  if (!ids.length) {
    ui.warning('请先勾选要审核的设计')
    return
  }
  if (ids.length > MAX_BATCH_REVIEW) {
    ui.error(`单次最多 ${MAX_BATCH_REVIEW} 条，请分批提交`)
    return
  }
  if (status === 'rejected' && !window.confirm(`将驳回 ${ids.length} 条设计，确认继续？`)) return

  batchBusy.value = true
  try {
    const result = await batchReviewRedstoneDesigns(ids, status, batchNote.value.trim())
    const action = status === 'published' ? '批量通过' : '批量驳回'
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
      <h1 class="text-xl font-bold text-text-primary md:text-2xl">红石设计审核</h1>
      <p class="mt-1 text-sm text-text-secondary">
        审核机器设计投稿：默认展示待审核项，通过后才会对公开接口可见；驳回需说明原因。审核人取自登录会话，
        审核状态与备注完整留痕并写入审计日志。
      </p>
    </header>

    <div class="card p-4">
      <div class="flex flex-wrap items-end gap-3">
        <select
          v-model="filters.reviewStatus"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="onFilter"
        >
          <option value="">全部审核状态</option>
          <option value="pending">{{ REVIEW_STATUS_LABELS.pending }}</option>
          <option value="published">{{ REVIEW_STATUS_LABELS.published }}</option>
          <option value="rejected">{{ REVIEW_STATUS_LABELS.rejected }}</option>
        </select>
        <select
          v-model="filters.purpose"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="onFilter"
        >
          <option value="">全部用途</option>
          <option v-for="p in REDSTONE_DESIGN_PURPOSES" :key="p" :value="p">
            {{ REDSTONE_DESIGN_PURPOSE_LABELS[p] }}
          </option>
        </select>
      </div>
    </div>

    <div v-if="pendingRows.length" class="card flex flex-wrap items-center gap-3 p-3 text-sm">
      <label class="flex items-center gap-2 text-text-secondary">
        <input
          type="checkbox"
          :checked="allSelected"
          class="h-4 w-4 accent-grass-500"
          @change="toggleAll"
        />
        全选本页待审核（{{ pendingRows.length }}）
      </label>
      <input
        v-model="batchNote"
        type="text"
        placeholder="批量审核备注（可选，会写入每一条）"
        class="h-8 w-64 rounded-md border border-border-dark bg-ink-800 px-2 text-xs text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
      />
      <button
        type="button"
        class="rounded border border-grass-500/50 bg-grass-500/10 px-3 py-1.5 text-xs text-grass-300 transition hover:bg-grass-500/20 disabled:opacity-40"
        :disabled="batchBusy || selected.size === 0"
        @click="batchReview('published')"
      >
        批量通过
      </button>
      <button
        type="button"
        class="rounded border border-danger-mc/50 bg-danger-mc/10 px-3 py-1.5 text-xs text-danger-mc transition hover:bg-danger-mc/20 disabled:opacity-40"
        :disabled="batchBusy || selected.size === 0"
        @click="batchReview('rejected')"
      >
        批量驳回
      </button>
      <span class="text-xs text-text-muted">
        已选 {{ selected.size }} 条{{ batchBusy ? ' · 处理中…' : '' }}
      </span>
    </div>

    <ListShell title="设计审核列表" :loading="loading" :error="error" @retry="load">
      <div class="card overflow-hidden">
        <div class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
              <tr>
                <th scope="col" class="w-10 px-4 py-3 font-medium">
                  <span class="sr-only">选择</span>
                </th>
                <th scope="col" class="px-4 py-3 font-medium">设计</th>
                <th scope="col" class="px-4 py-3 font-medium">用途 / 难度</th>
                <th scope="col" class="px-4 py-3 font-medium">作者 / 许可</th>
                <th scope="col" class="px-4 py-3 font-medium">来源</th>
                <th scope="col" class="px-4 py-3 font-medium">材料</th>
                <th scope="col" class="px-4 py-3 font-medium">审核状态</th>
                <th scope="col" class="px-4 py-3 font-medium">审核操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border-dark">
              <tr v-for="item in items" :key="item.id" class="align-top hover:bg-ink-800/60">
                <td class="px-4 py-3">
                  <input
                    v-if="item.review_status === 'pending'"
                    type="checkbox"
                    :checked="selected.has(item.id)"
                    class="mt-1 h-4 w-4 accent-grass-500"
                    :aria-label="`选择设计 ${item.name_zh || item.slug}`"
                    @change="toggleOne(item.id)"
                  />
                </td>
                <td class="px-4 py-3">
                  <div class="font-medium text-text-primary">{{ item.name_zh || '（未命名）' }}</div>
                  <div class="mt-0.5 font-mono text-xs text-text-muted">{{ item.slug }}</div>
                </td>
                <td class="px-4 py-3 text-text-secondary">
                  {{ REDSTONE_DESIGN_PURPOSE_LABELS[item.purpose] }}
                  <div class="mt-0.5 text-xs text-text-muted">{{ REDSTONE_DIFFICULTY_LABELS[item.difficulty] }}</div>
                </td>
                <td class="px-4 py-3 text-text-secondary">
                  {{ item.author || '—' }}
                  <div class="mt-0.5 text-xs text-text-muted">许可：{{ item.license || '未标注' }}</div>
                </td>
                <td class="px-4 py-3">
                  <a
                    v-if="isExternalUrl(item.source_url)"
                    :href="item.source_url || ''"
                    target="_blank"
                    rel="noopener noreferrer nofollow"
                    class="break-all font-mono text-xs text-grass-300 hover:underline"
                  >
                    {{ item.source_url }}
                  </a>
                  <span v-else class="text-xs text-text-muted">—</span>
                </td>
                <td class="px-4 py-3 text-text-secondary">
                  {{ item.materials.length }} 项
                  <div v-if="item.materials.length" class="mt-0.5 text-xs text-text-muted">
                    {{ joinList(item.materials.map((m) => m.name_zh || m.item_id)) }}
                  </div>
                </td>
                <td class="px-4 py-3">
                  <StatusBadge
                    :status="REVIEW_BADGE_STATUS[item.review_status]"
                    :label="REVIEW_STATUS_LABELS[item.review_status]"
                  />
                  <div v-if="item.reviewed_at" class="mt-1 text-xs text-text-muted">
                    最近审核：{{ item.reviewer || '（未记录审核人）' }} · {{ formatDateTime(item.reviewed_at) }}
                    <template v-if="item.review_note"> · {{ item.review_note }}</template>
                  </div>
                </td>
                <td class="px-4 py-3">
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
                      @click="review(item, 'published')"
                    >
                      通过
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
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <p v-if="!items.length" class="px-4 py-8 text-center text-sm text-text-muted">
          暂无设计——当前筛选下没有待审核的机器设计。
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
