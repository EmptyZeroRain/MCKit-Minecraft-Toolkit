<script setup lang="ts">
// 后台运维任务（gj.md 阶段二：任务重试与失败详情）
//
// 设计要点：
//   - 接口只负责入队，执行由服务端调度协程完成 → 前端轮询状态；
//   - 失败先自动重试（最多 max_attempts 次），耗尽后可由运营手动重试；
//   - 手动重试会新建任务并记录 retry_of，原始失败现场保留。
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import ListShell from '@/components/admin/list-shell.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useUiStore } from '@/stores/ui'
import {
  cancelJob,
  enqueueJob,
  listJobs,
  listJobTypes,
  retryJob,
} from '@/api/admin-jobs'
import { ApiError } from '@/types/api'
import {
  isJobActive,
  JOB_BADGE_STATUS,
  JOB_STATUS_LABELS,
  type AdminJob,
  type JobStatus,
  type JobTypeInfo,
} from '@/types/jobs'
import { formatDateTime } from '@/utils/format'

const ui = useUiStore()

const filters = reactive<{
  status: JobStatus | ''
  type: string
  page: number
  pageSize: number
}>({
  status: '',
  type: '',
  page: 1,
  pageSize: 20,
})

const items = ref<AdminJob[]>([])
const types = ref<JobTypeInfo[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')
/** 正在执行动作的任务 ID（防重复点击） */
const busyId = ref('')
/** 展开详情的任务 ID */
const expandedId = ref('')

// ===== 新建任务 =====
const enqueueType = ref('')
/** 通用 JSON 参数（未内置表单的任务类型使用） */
const enqueueJSON = ref('{}')
/** 内置表单：链接巡检 */
const linkOnlyPending = ref(true)
const linkLimit = ref(200)
const enqueueing = ref(false)

/** 是否使用内置表单（目前只有链接巡检有专用表单） */
const usingBuiltinForm = computed(() => enqueueType.value === 'linkhealth.run')

async function loadTypes() {
  try {
    types.value = await listJobTypes()
    if (!enqueueType.value && types.value.length) enqueueType.value = types.value[0].type
  } catch {
    // 类型列表失败不阻断主列表：仍可查看历史任务。
    types.value = []
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await listJobs({
      status: filters.status || undefined,
      type: filters.type || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    })
    items.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
    // 详情展开的记录如果已不在本页，收起。
    if (expandedId.value && !res.items.some((i) => i.id === expandedId.value)) {
      expandedId.value = ''
    }
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

/** 存在排队中 / 执行中的任务时自动刷新，终态后停止轮询。 */
const hasActive = computed(() => items.value.some(isJobActive))
let timer: ReturnType<typeof setInterval> | undefined

function schedulePoll() {
  if (timer) clearInterval(timer)
  timer = setInterval(() => {
    if (hasActive.value && !loading.value) load()
  }, 5000)
}

function onFilter() {
  filters.page = 1
  load()
}

function onPageChange(p: number) {
  filters.page = p
  load()
}

function toggleDetail(id: string) {
  expandedId.value = expandedId.value === id ? '' : id
}

async function enqueue() {
  if (enqueueing.value) return
  if (!enqueueType.value) {
    ui.warning('请选择任务类型')
    return
  }
  let payload: Record<string, unknown> = {}
  if (usingBuiltinForm.value) {
    payload = { only_pending: linkOnlyPending.value, limit: linkLimit.value }
    if (linkLimit.value < 0 || linkLimit.value > 2000) {
      ui.error('单次巡检条数需在 0–2000 之间（0 表示不限制）')
      return
    }
  } else {
    try {
      const parsed = JSON.parse(enqueueJSON.value || '{}')
      if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
        throw new Error('参数必须是 JSON 对象')
      }
      payload = parsed as Record<string, unknown>
    } catch (err) {
      ui.error(err instanceof Error ? `参数解析失败：${err.message}` : '参数解析失败')
      return
    }
  }

  enqueueing.value = true
  try {
    const job = await enqueueJob(enqueueType.value, payload)
    ui.success(`已入队：${typeLabel(job.type)}`)
    filters.page = 1
    await load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '入队失败')
  } finally {
    enqueueing.value = false
  }
}

async function retry(job: AdminJob) {
  if (busyId.value) return
  busyId.value = job.id
  try {
    await retryJob(job.id)
    ui.success(`已重新入队：${typeLabel(job.type)}`)
    await load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '重试失败')
  } finally {
    busyId.value = ''
  }
}

async function cancel(job: AdminJob) {
  if (busyId.value) return
  if (!window.confirm(`确认取消任务「${typeLabel(job.type)}」？`)) return
  busyId.value = job.id
  try {
    await cancelJob(job.id)
    ui.success('任务已取消')
    await load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '取消失败')
  } finally {
    busyId.value = ''
  }
}

function typeLabel(type: string): string {
  return types.value.find((t) => t.type === type)?.desc || type
}

/** 计算耗时（未完成返回 —）。 */
function duration(job: AdminJob): string {
  if (!job.started_at) return '—'
  const end = job.finished_at ? new Date(job.finished_at) : new Date()
  const ms = end.getTime() - new Date(job.started_at).getTime()
  if (!Number.isFinite(ms) || ms < 0) return '—'
  if (ms < 1000) return `${ms} ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)} 秒`
  return `${Math.round(ms / 60_000)} 分钟`
}

/** 结果摘要：单行展示关键计数，避免长 JSON 撑破表格。 */
function resultSummary(job: AdminJob): string {
  const entries = Object.entries(job.result || {})
  if (!entries.length) return '—'
  return entries
    .slice(0, 4)
    .map(([k, v]) => `${k}=${typeof v === 'object' ? JSON.stringify(v) : String(v)}`)
    .join(' · ')
}

function prettyJSON(value: unknown): string {
  try {
    return JSON.stringify(value ?? {}, null, 2)
  } catch {
    return String(value)
  }
}

const canRetry = (job: AdminJob) => job.status === 'failed' || job.status === 'cancelled'
const canCancel = (job: AdminJob) => job.status === 'pending'

onMounted(async () => {
  await Promise.all([loadTypes(), load()])
  schedulePoll()
})

onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div class="space-y-6">
    <header>
      <h1 class="text-xl font-bold text-text-primary md:text-2xl">后台任务</h1>
      <p class="mt-1 text-sm text-text-secondary">
        耗时的运维动作（如链接全量巡检）以任务形式异步执行：入队后立即返回，执行失败会自动重试
        （最多 {{ items[0]?.max_attempts ?? 3 }} 次），仍失败则保留错误详情，可人工重试或取消排队中的任务。
        人工重试会新建一条任务并保留原始失败现场。
      </p>
    </header>

    <div class="card p-4">
      <div class="flex flex-wrap items-end gap-3">
        <select
          v-model="filters.status"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="onFilter"
        >
          <option value="">全部状态</option>
          <option v-for="(label, key) in JOB_STATUS_LABELS" :key="key" :value="key">{{ label }}</option>
        </select>
        <select
          v-model="filters.type"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="onFilter"
        >
          <option value="">全部类型</option>
          <option v-for="t in types" :key="t.type" :value="t.type">{{ t.desc }}</option>
        </select>
        <button
          type="button"
          class="h-9 rounded-md border border-border-dark px-3 text-sm text-text-secondary transition hover:bg-ink-700 disabled:opacity-50"
          :disabled="loading"
          @click="load"
        >
          刷新
        </button>
        <span v-if="hasActive" class="text-xs text-text-muted">存在未完成任务，每 5 秒自动刷新</span>
      </div>

      <div class="mt-4 border-t border-border-dark pt-4">
        <h2 class="text-sm font-semibold text-text-primary">新建任务</h2>
        <div class="mt-2 flex flex-wrap items-end gap-3">
          <select
            v-model="enqueueType"
            class="h-9 min-w-48 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          >
            <option v-for="t in types" :key="t.type" :value="t.type">{{ t.desc }}</option>
          </select>

          <template v-if="usingBuiltinForm">
            <label class="flex items-center gap-2 text-xs text-text-secondary">
              <input v-model="linkOnlyPending" type="checkbox" class="h-4 w-4 accent-grass-500" />
              只复查未人工确认的链接
            </label>
            <label class="flex items-center gap-2 text-xs text-text-secondary">
              条数上限
              <input
                v-model.number="linkLimit"
                type="number"
                min="0"
                max="2000"
                class="h-9 w-24 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
              />
            </label>
          </template>
          <template v-else>
            <textarea
              v-model="enqueueJSON"
              rows="2"
              spellcheck="false"
              placeholder="任务参数（JSON 对象）"
              class="h-9 min-w-72 flex-1 rounded-md border border-border-dark bg-ink-800 px-2 py-1 font-mono text-xs text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
            />
          </template>

          <button
            type="button"
            class="h-9 rounded-md border border-grass-500/50 bg-grass-500/10 px-4 text-sm text-grass-300 transition hover:bg-grass-500/20 disabled:opacity-40"
            :disabled="enqueueing || !enqueueType"
            @click="enqueue"
          >
            入队
          </button>
        </div>
      </div>
    </div>

    <ListShell title="任务列表" :loading="loading" :error="error" @retry="load">
      <div class="card overflow-hidden">
        <div class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
              <tr>
                <th scope="col" class="px-4 py-3 font-medium">任务</th>
                <th scope="col" class="px-4 py-3 font-medium">状态</th>
                <th scope="col" class="px-4 py-3 font-medium">执行</th>
                <th scope="col" class="px-4 py-3 font-medium">结果 / 失败原因</th>
                <th scope="col" class="px-4 py-3 font-medium">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border-dark">
              <template v-for="job in items" :key="job.id">
                <tr class="align-top hover:bg-ink-800/60">
                  <td class="px-4 py-3">
                    <div class="font-medium text-text-primary">{{ typeLabel(job.type) }}</div>
                    <div class="mt-0.5 font-mono text-xs text-text-muted">{{ job.type }}</div>
                    <div class="mt-1 text-xs text-text-muted">
                      {{ job.created_by || '系统' }} · {{ formatDateTime(job.created_at) }}
                      <template v-if="job.retry_of"> · 重试自 {{ job.retry_of.slice(0, 8) }}…</template>
                    </div>
                  </td>
                  <td class="px-4 py-3">
                    <StatusBadge
                      :status="JOB_BADGE_STATUS[job.status]"
                      :label="JOB_STATUS_LABELS[job.status]"
                    />
                    <div class="mt-1 text-xs text-text-muted">
                      第 {{ job.attempts }} / {{ job.max_attempts }} 次
                    </div>
                  </td>
                  <td class="px-4 py-3 text-xs text-text-secondary">
                    <div>耗时：{{ duration(job) }}</div>
                    <div v-if="job.started_at" class="mt-0.5 text-text-muted">
                      开始 {{ formatDateTime(job.started_at) }}
                    </div>
                  </td>
                  <td class="px-4 py-3 text-xs">
                    <p v-if="job.last_error" class="max-w-md text-danger-mc">
                      {{ job.last_error }}
                    </p>
                    <p v-else class="max-w-md text-text-secondary">{{ resultSummary(job) }}</p>
                  </td>
                  <td class="px-4 py-3">
                    <div class="flex flex-wrap gap-2">
                      <button
                        type="button"
                        class="rounded border border-border-dark px-2 py-1 text-xs text-text-secondary transition hover:bg-ink-700"
                        @click="toggleDetail(job.id)"
                      >
                        {{ expandedId === job.id ? '收起' : '详情' }}
                      </button>
                      <button
                        v-if="canRetry(job)"
                        type="button"
                        class="rounded border border-grass-500/50 bg-grass-500/10 px-2 py-1 text-xs text-grass-300 transition hover:bg-grass-500/20 disabled:opacity-50"
                        :disabled="busyId === job.id"
                        @click="retry(job)"
                      >
                        重试
                      </button>
                      <button
                        v-if="canCancel(job)"
                        type="button"
                        class="rounded border border-danger-mc/50 bg-danger-mc/10 px-2 py-1 text-xs text-danger-mc transition hover:bg-danger-mc/20 disabled:opacity-50"
                        :disabled="busyId === job.id"
                        @click="cancel(job)"
                      >
                        取消
                      </button>
                    </div>
                  </td>
                </tr>
                <tr v-if="expandedId === job.id" class="bg-ink-900/60">
                  <td colspan="5" class="px-4 py-3">
                    <div class="grid gap-3 md:grid-cols-2">
                      <div>
                        <div class="mb-1 text-xs font-medium text-text-muted">任务参数</div>
                        <pre class="max-h-48 overflow-auto rounded bg-ink-950 p-2 font-mono text-xs text-text-secondary">{{ prettyJSON(job.payload) }}</pre>
                      </div>
                      <div>
                        <div class="mb-1 text-xs font-medium text-text-muted">执行结果</div>
                        <pre class="max-h-48 overflow-auto rounded bg-ink-950 p-2 font-mono text-xs text-text-secondary">{{ prettyJSON(job.result) }}</pre>
                      </div>
                    </div>
                    <p v-if="job.last_error" class="mt-2 text-xs text-danger-mc">
                      失败原因：{{ job.last_error }}
                    </p>
                    <p class="mt-2 font-mono text-xs text-text-muted">ID：{{ job.id }}</p>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
        <p v-if="!items.length" class="px-4 py-8 text-center text-sm text-text-muted">
          暂无任务——可在上方选择类型后入队。
        </p>

        <div class="px-4 pb-4">
          <Pagination
            :page="filters.page"
            :page-size="filters.pageSize"
            :total="total"
            :total-pages="totalPages"
            @change="onPageChange"
          />
        </div>
      </div>
    </ListShell>
  </div>
</template>
