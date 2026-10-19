<script setup lang="ts">
// 内容建设任务（gj.md §8.3 搜索词闭环 / §15 内容任务）
//
// 需 ops.contenttasks 权限（侧边栏过滤 + 后端 403 兜底）。
// 闭环：无结果搜索词达到阈值 → 创建任务并指派负责人 → 新增内容后「重新检测」
// 搜索结果 → 有结果即标记完成。任务只涉及聚合关键词与账号指派，不含用户标识。
import { computed, onMounted, reactive, ref } from 'vue'
import ListShell from '@/components/admin/list-shell.vue'
import AdminTable from '@/components/admin/AdminTable.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useUiStore } from '@/stores/ui'
import { listAdmins } from '@/api/admin-admins'
import {
  createContentTask,
  deleteContentTask,
  listContentTaskCandidates,
  listContentTasks,
  recheckContentTask,
  updateContentTask,
} from '@/api/admin-content-tasks'
import type { AdminTableColumn, AdminUser } from '@/types/admin'
import { ApiError } from '@/types/api'
import {
  CONTENT_TASK_CONTENT_LABELS,
  CONTENT_TASK_STATUS_CLASSES,
  CONTENT_TASK_STATUS_LABELS,
  CONTENT_TASK_STATUS_OPTIONS,
} from '@/types/content-task'
import type {
  ContentTask,
  ContentTaskCandidate,
  ContentTaskContentType,
  ContentTaskRecheckResult,
  ContentTaskStatus,
} from '@/types/content-task'
import { timeAgo } from '@/utils/format'

const ui = useUiStore()

const filters = reactive<{ status: ContentTaskStatus | ''; q: string; page: number; pageSize: number }>(
  { status: '', q: '', page: 1, pageSize: 20 },
)

const tasks = ref<ContentTask[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')

const candidates = ref<ContentTaskCandidate[]>([])
const candidatesLoading = ref(false)
const candidatesError = ref('')

// 可指派的后台账号（role <> 'user'）
const admins = ref<AdminUser[]>([])

// 行内操作进行中标记（避免重复点击）
const busyId = ref('')
// 从候选创建任务时的关键词（用于按钮 loading）
const creatingTerm = ref('')

const toDelete = ref<ContentTask | null>(null)
const deleting = ref(false)

// 编辑弹窗
const editing = ref<ContentTask | null>(null)
const saving = ref(false)
const rechecking = ref(false)
const recheckResult = ref<ContentTaskRecheckResult | null>(null)
const editForm = reactive<{
  status: ContentTaskStatus
  assignee_id: string
  related_content_type: ContentTaskContentType
  related_content_id: string
  note: string
}>({ status: 'open', assignee_id: '', related_content_type: '', related_content_id: '', note: '' })

const statusOptions = CONTENT_TASK_STATUS_OPTIONS

const candidateColumns: AdminTableColumn[] = [
  { key: 'term', label: '关键词' },
  { key: 'hits', label: '搜索次数', width: '110px' },
  { key: 'no_result_hits', label: '无结果次数', width: '110px' },
  { key: 'last_seen_at', label: '最近搜索', width: '120px' },
  { key: 'action', label: '操作', align: 'right', width: '130px' },
]

const taskColumns: AdminTableColumn[] = [
  { key: 'keyword', label: '关键词' },
  { key: 'counts', label: '搜索 / 无结果', width: '130px' },
  { key: 'assignee', label: '负责人', width: '130px' },
  { key: 'status', label: '状态', width: '120px' },
  { key: 'related', label: '关联内容', width: '140px' },
  { key: 'updated_at', label: '最近更新', width: '110px' },
  { key: 'action', label: '操作', align: 'right', width: '180px' },
]

/** 状态徽标类名。 */
function statusClass(status: ContentTaskStatus): string {
  return CONTENT_TASK_STATUS_CLASSES[status] ?? CONTENT_TASK_STATUS_CLASSES.open
}

/** 关联内容的编辑页路径（用于跳转核对）。 */
function contentEditPath(type: ContentTaskContentType, id: string | null): string {
  if (!id) return ''
  if (type === 'resource') return `/admin/resources/${id}/edit`
  if (type === 'tool') return `/admin/tools/${id}/edit`
  if (type === 'wiki') return `/admin/wiki/${id}/edit`
  return ''
}

async function loadTasks() {
  loading.value = true
  error.value = ''
  try {
    const res = await listContentTasks({
      status: filters.status || undefined,
      q: filters.q.trim() || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    })
    tasks.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function loadCandidates() {
  candidatesLoading.value = true
  candidatesError.value = ''
  try {
    candidates.value = await listContentTaskCandidates(20)
  } catch (err) {
    candidatesError.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    candidatesLoading.value = false
  }
}

// 拉取可指派账号；失败不阻断任务列表（仅负责人下拉为空）。
async function loadAdmins() {
  try {
    const res = await listAdmins({ page: 1, page_size: 100 })
    admins.value = res.items
  } catch {
    admins.value = []
  }
}

async function refreshAll() {
  await Promise.all([loadTasks(), loadCandidates()])
}

function onPageChange(p: number) {
  filters.page = p
  loadTasks()
}

function onFilterChange() {
  filters.page = 1
  loadTasks()
}

/** 从无结果候选一键创建任务。 */
async function onCreateFromCandidate(candidate: ContentTaskCandidate) {
  if (candidate.task_id) return
  creatingTerm.value = candidate.term
  try {
    await createContentTask({ keyword: candidate.term })
    ui.success(`已为「${candidate.term}」创建内容建设任务`)
    await refreshAll()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '创建任务失败')
  } finally {
    creatingTerm.value = ''
  }
}

/** 表格内快速切换状态。 */
async function onStatusChange(task: ContentTask, event: Event) {
  const next = (event.target as HTMLSelectElement).value as ContentTaskStatus
  if (next === task.status) return
  busyId.value = task.id
  try {
    await updateContentTask({ id: task.id, status: next })
    ui.success(`「${task.keyword}」已标记为${CONTENT_TASK_STATUS_LABELS[next]}`)
    await loadTasks()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '更新状态失败')
    // 回滚下拉显示（重新拉取会覆盖，此处仅在失败时恢复原值）
    ;(event.target as HTMLSelectElement).value = task.status
  } finally {
    busyId.value = ''
  }
}

function openEdit(task: ContentTask) {
  editing.value = task
  recheckResult.value = null
  editForm.status = task.status
  editForm.assignee_id = task.assignee_id ?? ''
  editForm.related_content_type = task.related_content_type
  editForm.related_content_id = task.related_content_id ?? ''
  editForm.note = task.note
}

function closeEdit() {
  editing.value = null
  recheckResult.value = null
}

/** 保存编辑：仅提交发生变化的字段，避免覆盖他人并发修改。 */
async function saveEdit() {
  const task = editing.value
  if (!task) return
  const payload: {
    id: string
    status?: ContentTaskStatus
    assignee_id?: string
    related_content_type?: ContentTaskContentType
    related_content_id?: string
    note?: string
  } = { id: task.id }

  if (editForm.status !== task.status) payload.status = editForm.status
  if (editForm.assignee_id !== (task.assignee_id ?? '')) {
    payload.assignee_id = editForm.assignee_id
  }
  if (editForm.related_content_type !== task.related_content_type) {
    payload.related_content_type = editForm.related_content_type
  }
  if (editForm.related_content_id !== (task.related_content_id ?? '')) {
    payload.related_content_id = editForm.related_content_id
  }
  if (editForm.note !== task.note) payload.note = editForm.note

  if (Object.keys(payload).length === 1) {
    ui.info('没有需要保存的修改')
    return
  }

  saving.value = true
  try {
    await updateContentTask(payload)
    ui.success('任务已更新')
    closeEdit()
    await loadTasks()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '保存失败')
  } finally {
    saving.value = false
  }
}

/** 重新检测搜索结果；有结果时提示可标记完成。 */
async function doRecheck() {
  const task = editing.value
  if (!task) return
  rechecking.value = true
  try {
    const res = await recheckContentTask(task.id)
    recheckResult.value = res
    if (res.has_results) {
      ui.success(`「${res.keyword}」已有 ${res.result_count} 条结果，可标记完成`)
    } else {
      ui.warning(`「${res.keyword}」仍无搜索结果，需继续补充内容`)
    }
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '重新检测失败')
  } finally {
    rechecking.value = false
  }
}

/** 重新检测有结果后一键完成。 */
async function markDone() {
  const task = editing.value
  if (!task) return
  editForm.status = 'done'
  await saveEdit()
}

async function confirmDelete() {
  const task = toDelete.value
  if (!task) return
  deleting.value = true
  try {
    await deleteContentTask(task.id)
    ui.success('任务已删除')
    toDelete.value = null
    await refreshAll()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '删除失败')
  } finally {
    deleting.value = false
  }
}

const openCandidateCount = computed(
  () => candidates.value.filter((c) => !c.task_id).length,
)

onMounted(() => {
  loadTasks()
  loadCandidates()
  loadAdmins()
})
</script>

<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between gap-4">
      <div>
        <h1 class="text-lg font-semibold text-text-primary">内容建设任务</h1>
        <p class="mt-1 text-sm text-text-muted">
          把高频无结果搜索词沉淀为可指派、可跟踪的运营任务；新增内容后重新检测结果并标记完成。
        </p>
      </div>
      <button
        type="button"
        class="shrink-0 rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-primary hover:bg-ink-600 disabled:opacity-50"
        :disabled="loading || candidatesLoading"
        @click="refreshAll"
      >
        刷新
      </button>
    </div>

    <ListShell
      title="搜索缺口候选"
      :loading="candidatesLoading"
      :error="candidatesError"
      @retry="loadCandidates"
    >
      <p class="mb-3 text-xs text-text-muted">
        无结果次数达到阈值且尚无未完结任务的关键词；当前有
        <span class="text-amber-300">{{ openCandidateCount }}</span>
        个可转为任务。
      </p>
      <AdminTable
        :columns="candidateColumns"
        :rows="candidates"
        row-key="term"
        min-width="720px"
        empty-text="暂无达到阈值的无结果词，资料覆盖良好。"
      >
        <template #cell-no_result_hits="{ row }">
          <span class="text-amber-300">{{ row.no_result_hits }}</span>
        </template>
        <template #cell-last_seen_at="{ row }">
          <span class="text-xs text-text-muted">{{ timeAgo(row.last_seen_at) }}</span>
        </template>
        <template #cell-action="{ row }">
          <span v-if="row.task_id" class="text-xs text-text-muted">已有任务</span>
          <button
            v-else
            type="button"
            class="rounded-md border border-border-light bg-ink-700 px-3 py-1 text-xs text-text-primary hover:bg-ink-600 disabled:opacity-50"
            :disabled="creatingTerm === row.term"
            @click="onCreateFromCandidate(row)"
          >
            {{ creatingTerm === row.term ? '创建中…' : '转为任务' }}
          </button>
        </template>
      </AdminTable>
    </ListShell>

    <ListShell title="任务列表" :loading="loading" :error="error" @retry="loadTasks">
      <div class="mb-4 flex flex-wrap items-center gap-3">
        <select
          v-model="filters.status"
          aria-label="按状态筛选"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="onFilterChange"
        >
          <option value="">全部状态</option>
          <option v-for="opt in statusOptions" :key="opt.value" :value="opt.value">
            {{ opt.label }}
          </option>
        </select>
        <input
          v-model="filters.q"
          type="search"
          maxlength="64"
          placeholder="过滤关键词…"
          aria-label="过滤关键词"
          class="h-9 w-full max-w-xs rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          @change="onFilterChange"
        />
      </div>

      <AdminTable
        :columns="taskColumns"
        :rows="tasks"
        row-key="id"
        min-width="900px"
        empty-text="暂无内容建设任务。"
      >
        <template #cell-keyword="{ row }">
          <span class="text-text-primary">{{ row.keyword }}</span>
        </template>
        <template #cell-counts="{ row }">
          <span class="text-text-secondary">{{ row.hits }}</span>
          <span class="text-text-muted"> / </span>
          <span class="text-amber-300">{{ row.no_result_hits }}</span>
        </template>
        <template #cell-assignee="{ row }">
          <span :class="row.assignee_name ? 'text-text-secondary' : 'text-text-muted'">
            {{ row.assignee_name || '未指派' }}
          </span>
        </template>
        <template #cell-status="{ row }">
          <div class="flex items-center gap-2">
            <span
              class="rounded px-1.5 py-0.5 text-xs"
              :class="statusClass(row.status)"
            >
              {{ CONTENT_TASK_STATUS_LABELS[row.status] }}
            </span>
            <select
              :value="row.status"
              :disabled="busyId === row.id"
              :aria-label="`修改「${row.keyword}」状态`"
              class="h-7 rounded border border-border-dark bg-ink-800 px-1 text-xs text-text-primary focus:border-grass-500 focus:outline-none disabled:opacity-50"
              @change="onStatusChange(row, $event)"
            >
              <option v-for="opt in statusOptions" :key="opt.value" :value="opt.value">
                {{ opt.label }}
              </option>
            </select>
          </div>
        </template>
        <template #cell-related="{ row }">
          <a
            v-if="row.related_content_type && row.related_content_id"
            :href="contentEditPath(row.related_content_type, row.related_content_id)"
            class="text-xs text-grass-400 hover:underline"
          >
            {{ CONTENT_TASK_CONTENT_LABELS[row.related_content_type] }}
          </a>
          <span v-else class="text-xs text-text-muted">—</span>
        </template>
        <template #cell-updated_at="{ row }">
          <span class="text-xs text-text-muted">{{ timeAgo(row.updated_at) }}</span>
        </template>
        <template #cell-action="{ row }">
          <div class="flex items-center justify-end gap-2">
            <button
              type="button"
              class="rounded-md border border-border-light bg-ink-700 px-3 py-1 text-xs text-text-primary hover:bg-ink-600"
              @click="openEdit(row)"
            >
              编辑
            </button>
            <button
              type="button"
              class="rounded-md border border-danger-mc/40 px-3 py-1 text-xs text-danger-mc hover:bg-danger-mc/10"
              @click="toDelete = row"
            >
              删除
            </button>
          </div>
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

    <!-- 编辑弹窗 -->
    <Teleport to="body">
      <div
        v-if="editing"
        class="fixed inset-0 z-[90] flex items-center justify-center bg-black/60 p-4"
        role="dialog"
        aria-modal="true"
        aria-label="编辑内容建设任务"
        @click.self="closeEdit"
      >
        <div class="card max-h-[85vh] w-full max-w-lg overflow-y-auto p-6 shadow-2xl">
          <h2 class="mb-1 text-lg font-semibold text-text-primary">
            编辑任务：{{ editing.keyword }}
          </h2>
          <p class="mb-4 text-xs text-text-muted">
            搜索 {{ editing.hits }} 次 / 无结果 {{ editing.no_result_hits }} 次；
            创建于 {{ timeAgo(editing.created_at) }}
          </p>

          <div class="space-y-4">
            <label class="block">
              <span class="mb-1 block text-xs text-text-muted">处理状态</span>
              <select
                v-model="editForm.status"
                class="h-9 w-full rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
              >
                <option v-for="opt in statusOptions" :key="opt.value" :value="opt.value">
                  {{ opt.label }}
                </option>
              </select>
            </label>

            <label class="block">
              <span class="mb-1 block text-xs text-text-muted">负责人</span>
              <select
                v-model="editForm.assignee_id"
                class="h-9 w-full rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
              >
                <option value="">未指派</option>
                <option v-for="a in admins" :key="a.id" :value="a.id">
                  {{ a.nickname || a.username }}
                </option>
              </select>
            </label>

            <div class="grid grid-cols-2 gap-3">
              <label class="block">
                <span class="mb-1 block text-xs text-text-muted">关联内容类型</span>
                <select
                  v-model="editForm.related_content_type"
                  class="h-9 w-full rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
                >
                  <option value="">未关联</option>
                  <option value="resource">{{ CONTENT_TASK_CONTENT_LABELS.resource }}</option>
                  <option value="tool">{{ CONTENT_TASK_CONTENT_LABELS.tool }}</option>
                  <option value="wiki">{{ CONTENT_TASK_CONTENT_LABELS.wiki }}</option>
                </select>
              </label>
              <label class="block">
                <span class="mb-1 block text-xs text-text-muted">关联内容 ID</span>
                <input
                  v-model="editForm.related_content_id"
                  type="text"
                  placeholder="内容 UUID"
                  class="h-9 w-full rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
                />
              </label>
            </div>

            <label class="block">
              <span class="mb-1 block text-xs text-text-muted">备注</span>
              <textarea
                v-model="editForm.note"
                rows="3"
                maxlength="2000"
                placeholder="处理进展、补充内容说明…"
                class="w-full rounded-md border border-border-dark bg-ink-800 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
              ></textarea>
            </label>

            <div
              v-if="recheckResult"
              class="rounded-md border p-3 text-xs"
              :class="
                recheckResult.has_results
                  ? 'border-grass-500/40 bg-grass-500/10 text-grass-300'
                  : 'border-amber-500/40 bg-amber-500/10 text-amber-300'
              "
            >
              <template v-if="recheckResult.has_results">
                检测到 {{ recheckResult.result_count }} 条搜索结果，可将任务标记为已完成。
              </template>
              <template v-else> 仍无搜索结果，请继续补充内容后再检测。 </template>
            </div>
          </div>

          <div class="mt-6 flex flex-wrap items-center justify-between gap-3">
            <div class="flex gap-2">
              <button
                type="button"
                class="rounded-md border border-border-light bg-ink-700 px-3 py-2 text-sm text-text-primary hover:bg-ink-600 disabled:opacity-50"
                :disabled="rechecking"
                @click="doRecheck"
              >
                {{ rechecking ? '检测中…' : '重新检测' }}
              </button>
              <button
                v-if="recheckResult?.has_results && editForm.status !== 'done'"
                type="button"
                class="rounded-md border border-grass-500/50 bg-grass-500/15 px-3 py-2 text-sm text-grass-300 hover:bg-grass-500/25 disabled:opacity-50"
                :disabled="saving"
                @click="markDone"
              >
                标记完成
              </button>
            </div>
            <div class="flex gap-2">
              <button
                type="button"
                class="rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-primary hover:bg-ink-600"
                @click="closeEdit"
              >
                取消
              </button>
              <button
                type="button"
                class="rounded-md bg-grass-600 px-4 py-2 text-sm text-white hover:bg-grass-500 disabled:opacity-50"
                :disabled="saving"
                @click="saveEdit"
              >
                {{ saving ? '保存中…' : '保存' }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </Teleport>

    <ConfirmDialog
      :open="!!toDelete"
      title="删除内容建设任务"
      :message="`确认删除「${toDelete?.keyword ?? ''}」的任务？删除后不影响搜索统计，仍可重新创建。`"
      confirm-text="确认删除"
      dangerous
      :confirm-disabled="deleting"
      @confirm="confirmDelete"
      @cancel="toDelete = null"
    />
  </div>
</template>
