<script setup lang="ts">
// 数据质量监控（TODO §2 版本化资料规则 / §4 数据导入与质量监控）
// 仅 admin 角色可见（侧边栏过滤 + 后端 403 兜底）。
import { computed, onMounted, reactive, ref } from 'vue'
import ListShell from '@/components/admin/list-shell.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useUiStore } from '@/stores/ui'
import {
  fetchDataQualityOverview,
  listDataImportJobs,
  listQualityAssignees,
  updateQualityIssue,
} from '@/api/admin-data-quality'
import { ApiError } from '@/types/api'
import type {
  DataImportJob,
  DataQualityCheck,
  DataQualityOverview,
  QualityAssignee,
  QualityIssueStatus,
} from '@/types/data-quality'
import {
  actionableChecks,
  categoryLabel,
  checkActionPath,
  groupChecksByCategory,
  isIssueClosed,
  issueStatusClass,
  ISSUE_STATUS_LABELS,
  isJobFailed,
  problemRatio,
  severityClass,
  severityLabel,
} from '@/utils/data-quality'
import { timeAgo } from '@/utils/format'

const ui = useUiStore()

// ---- 概览 ----
const overview = ref<DataQualityOverview | null>(null)
const loading = ref(false)
const error = ref('')

// ---- 导入任务 ----
const jobFilters = reactive<{ dataType: string; page: number; pageSize: number }>({
  dataType: '',
  page: 1,
  pageSize: 15,
})
const jobs = ref<DataImportJob[]>([])
const jobsTotal = ref(0)
const jobsTotalPages = ref(1)
const jobsLoading = ref(false)
const jobsError = ref('')

// 需要关注的检查项（严重/提醒且命中数 > 0）
const actionable = computed(() => (overview.value ? actionableChecks(overview.value.checks) : []))
// 按类别分组的全部检查项
const grouped = computed(() => (overview.value ? groupChecksByCategory(overview.value.checks) : []))

async function loadOverview() {
  loading.value = true
  error.value = ''
  try {
    overview.value = await fetchDataQualityOverview()
    syncIssueDrafts(overview.value)
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function loadJobs() {
  jobsLoading.value = true
  jobsError.value = ''
  try {
    const res = await listDataImportJobs({
      data_type: jobFilters.dataType || undefined,
      page: jobFilters.page,
      page_size: jobFilters.pageSize,
    })
    jobs.value = res.items
    jobsTotal.value = res.total
    jobsTotalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / jobFilters.pageSize))
  } catch (err) {
    jobsError.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    jobsLoading.value = false
  }
}

function onJobPageChange(p: number) {
  jobFilters.page = p
  loadJobs()
}

function onJobFilter() {
  jobFilters.page = 1
  loadJobs()
}

function reloadAll() {
  loadOverview()
  loadJobs()
  ui.success('已刷新')
}

// ---- 数据质量问题工单（分派 / 处理状态 / 直接跳转） ----
const assignees = ref<QualityAssignee[]>([])
/** 各检查项的负责人草稿（未选择时为空串） */
const issueAssignee = reactive<Record<string, string>>({})
/** 各检查项的处理备注草稿 */
const issueNote = reactive<Record<string, string>>({})
/** 正在保存的检查项 key */
const issueBusy = ref('')

async function loadAssignees() {
  try {
    assignees.value = await listQualityAssignees()
  } catch {
    // 负责人列表失败不阻断概览：仍可查看问题与处理状态。
    assignees.value = []
  }
}

/** 用后端返回的工单状态初始化草稿。 */
function syncIssueDrafts(ov: DataQualityOverview | null) {
  if (!ov) return
  for (const c of ov.checks) {
    if (!(c.key in issueAssignee)) issueAssignee[c.key] = c.issue?.assignee_id ?? ''
    if (!(c.key in issueNote)) issueNote[c.key] = c.issue?.note ?? ''
  }
}

async function saveIssue(check: DataQualityCheck, status: QualityIssueStatus) {
  if (issueBusy.value) return
  issueBusy.value = check.key
  try {
    await updateQualityIssue({
      check_key: check.key,
      status,
      assignee_id: issueAssignee[check.key] || undefined,
      note: issueNote[check.key] || '',
      snapshot_count: check.count,
    })
    const action =
      status === 'assigned' ? '已指派' : status === 'resolved' ? '已标记解决' : status === 'ignored' ? '已忽略' : '已重新打开'
    ui.success(`${action}：${check.label}`)
    await loadOverview()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '保存处理状态失败')
  } finally {
    issueBusy.value = ''
  }
}

/** 切换负责人即保存（空值表示清除指派）。 */
function onAssigneeChange(check: DataQualityCheck) {
  const status: QualityIssueStatus = issueAssignee[check.key] ? 'assigned' : 'open'
  saveIssue(check, status)
}

/** 备注在失焦时保存，状态沿用当前工单状态（未建单视为待处理）。 */
function saveNote(check: DataQualityCheck) {
  const status = check.issue?.status ?? 'open'
  if (isIssueClosed(status) && !issueNote[check.key]) return
  saveIssue(check, status)
}

// 导入任务失败行的展开开关
const expanded = ref('')
function toggleExpand(id: string) {
  expanded.value = expanded.value === id ? '' : id
}

onMounted(() => {
  loadAssignees()
  reloadAll()
})
</script>

<template>
  <div class="space-y-5">
    <div class="flex items-center justify-between gap-4">
      <div>
        <h1 class="text-lg font-semibold text-text-primary">数据质量监控</h1>
        <p class="mt-1 text-sm text-text-muted">
          客观检查资料库的缺失、非法格式与悬空引用；导入任务记录每次数据同步的来源与结果。
        </p>
      </div>
      <button
        type="button"
        class="shrink-0 rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-primary hover:bg-ink-600 disabled:opacity-50"
        :disabled="loading"
        @click="reloadAll"
      >
        刷新
      </button>
    </div>

    <ListShell title="质量概览" :loading="loading" :error="error" @retry="loadOverview">
      <template v-if="overview">
        <!-- 汇总卡片 -->
        <div class="mb-5 grid gap-4 sm:grid-cols-3">
          <div class="card p-4">
            <div class="text-xs text-text-muted">严重问题项</div>
            <div class="mt-1 text-2xl font-semibold" :class="overview.error_checks ? 'text-danger-mc' : 'text-grass-300'">
              {{ overview.error_checks }}
            </div>
          </div>
          <div class="card p-4">
            <div class="text-xs text-text-muted">提醒项</div>
            <div class="mt-1 text-2xl font-semibold" :class="overview.warning_checks ? 'text-amber-300' : 'text-grass-300'">
              {{ overview.warning_checks }}
            </div>
          </div>
          <div class="card p-4">
            <div class="text-xs text-text-muted">检查时间</div>
            <div class="mt-1 text-sm text-text-secondary">{{ timeAgo(overview.generated_at) }}</div>
          </div>
        </div>

        <!-- 需要关注的检查项（含分派 / 处理状态 / 直接跳转） -->
        <div v-if="actionable.length" class="mb-5 space-y-2">
          <div
            v-for="c in actionable"
            :key="c.key"
            class="rounded-md border border-border-dark bg-ink-900 p-3"
          >
            <div class="flex items-start gap-3">
              <span class="mt-0.5 shrink-0 rounded px-2 py-0.5 text-xs" :class="severityClass(c.severity)">
                {{ severityLabel(c.severity) }}
              </span>
              <div class="min-w-0 flex-1">
                <div class="flex flex-wrap items-center gap-2 text-sm text-text-primary">
                  <span>{{ categoryLabel(c.category) }} · {{ c.label }}</span>
                  <span class="text-text-muted">
                    {{ c.count }}<span v-if="c.total"> / {{ c.total }}（{{ problemRatio(c.count, c.total) }}%）</span>
                  </span>
                  <span
                    v-if="c.issue"
                    class="rounded px-2 py-0.5 text-xs"
                    :class="issueStatusClass(c.issue.status)"
                  >
                    {{ ISSUE_STATUS_LABELS[c.issue.status] }}
                  </span>
                  <span
                    v-else
                    class="rounded bg-amber-500/15 px-2 py-0.5 text-xs text-amber-300"
                  >
                    {{ ISSUE_STATUS_LABELS.open }}
                  </span>
                </div>
                <p class="mt-0.5 text-xs text-text-muted">{{ c.hint }}</p>
                <div v-if="c.issue?.updated_at" class="mt-1 text-xs text-text-muted">
                  {{ c.issue.assignee_name ? `负责人：${c.issue.assignee_name} · ` : '' }}
                  最近更新：{{ c.issue.updated_by || '系统' }} · {{ timeAgo(c.issue.updated_at) }}
                  <template v-if="c.issue.snapshot_count !== c.count">
                    · 处理时 {{ c.issue.snapshot_count }} 条
                  </template>
                </div>

                <div class="mt-2 flex flex-wrap items-center gap-2">
                  <select
                    v-model="issueAssignee[c.key]"
                    aria-label="问题负责人"
                    class="h-8 rounded border border-border-dark bg-ink-800 px-2 text-xs text-text-primary focus:border-grass-500 focus:outline-none"
                    :disabled="issueBusy === c.key"
                    @change="onAssigneeChange(c)"
                  >
                    <option value="">未指派</option>
                    <option v-for="a in assignees" :key="a.id" :value="a.id">{{ a.name }}</option>
                  </select>
                  <input
                    v-model="issueNote[c.key]"
                    type="text"
                    placeholder="处理备注（失焦保存）"
                    class="h-8 w-56 rounded border border-border-dark bg-ink-800 px-2 text-xs text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
                    :disabled="issueBusy === c.key"
                    @blur="saveNote(c)"
                  />
                  <button
                    v-if="!isIssueClosed(c.issue?.status)"
                    type="button"
                    class="rounded border border-grass-500/50 bg-grass-500/10 px-2 py-1 text-xs text-grass-300 transition hover:bg-grass-500/20 disabled:opacity-50"
                    :disabled="issueBusy === c.key"
                    @click="saveIssue(c, 'resolved')"
                  >
                    标记已解决
                  </button>
                  <button
                    v-if="!isIssueClosed(c.issue?.status)"
                    type="button"
                    class="rounded border border-border-dark px-2 py-1 text-xs text-text-secondary transition hover:bg-ink-700 disabled:opacity-50"
                    :disabled="issueBusy === c.key"
                    @click="saveIssue(c, 'ignored')"
                  >
                    忽略
                  </button>
                  <button
                    v-if="isIssueClosed(c.issue?.status)"
                    type="button"
                    class="rounded border border-border-dark px-2 py-1 text-xs text-text-secondary transition hover:bg-ink-700 disabled:opacity-50"
                    :disabled="issueBusy === c.key"
                    @click="saveIssue(c, 'open')"
                  >
                    重新打开
                  </button>
                  <RouterLink
                    v-if="checkActionPath(c.category)"
                    :to="checkActionPath(c.category)"
                    class="text-xs text-grass-300 hover:underline"
                  >
                    去处理 →
                  </RouterLink>
                </div>
              </div>
            </div>
          </div>
        </div>
        <div v-else class="mb-5 rounded-md border border-grass-600/30 bg-grass-600/10 p-3 text-sm text-grass-300">
          未发现需要处理的数据质量问题。
        </div>

        <!-- 全部检查项 -->
        <div class="space-y-4">
          <div v-for="g in grouped" :key="g.category">
            <h3 class="mb-2 text-sm font-semibold text-text-secondary">{{ categoryLabel(g.category) }}</h3>
            <div class="card overflow-x-auto">
              <table class="w-full min-w-[640px] text-left text-sm">
                <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
                  <tr>
                    <th scope="col" class="px-4 py-2.5">检查项</th>
                    <th scope="col" class="px-4 py-2.5">级别</th>
                    <th scope="col" class="px-4 py-2.5">命中 / 总数</th>
                    <th scope="col" class="px-4 py-2.5">说明</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-border-dark">
                  <tr v-for="c in g.checks" :key="c.key" class="hover:bg-ink-800/60">
                    <td class="px-4 py-2.5 text-text-primary">{{ c.label }}</td>
                    <td class="px-4 py-2.5">
                      <span class="rounded px-2 py-0.5 text-xs" :class="severityClass(c.severity)">
                        {{ severityLabel(c.severity) }}
                      </span>
                    </td>
                    <td class="px-4 py-2.5 text-text-secondary">
                      {{ c.count }}<span v-if="c.total"> / {{ c.total }}</span>
                    </td>
                    <td class="px-4 py-2.5 text-xs text-text-muted">{{ c.hint }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </template>
    </ListShell>

    <!-- 导入任务 -->
    <div id="import-jobs" />
    <ListShell title="数据导入任务" :loading="jobsLoading" :error="jobsError" @retry="loadJobs">
      <div class="mb-4">
        <select
          v-model="jobFilters.dataType"
          aria-label="按数据类型过滤"
          class="h-9 w-full max-w-xs rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="onJobFilter"
        >
          <option value="">全部类型</option>
          <option value="items">items</option>
          <option value="recipes">recipes</option>
          <option value="commands">commands</option>
          <option value="tools">tools</option>
        </select>
      </div>

      <div v-if="jobs.length" class="card overflow-x-auto">
        <table class="w-full min-w-[860px] text-left text-sm">
          <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
            <tr>
              <th scope="col" class="px-4 py-2.5">时间</th>
              <th scope="col" class="px-4 py-2.5">类型</th>
              <th scope="col" class="px-4 py-2.5">来源文件</th>
              <th scope="col" class="px-4 py-2.5">总数</th>
              <th scope="col" class="px-4 py-2.5">新增 / 更新</th>
              <th scope="col" class="px-4 py-2.5">失败</th>
              <th scope="col" class="px-4 py-2.5">执行者</th>
              <th scope="col" class="px-4 py-2.5 text-right">详情</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border-dark">
            <template v-for="job in jobs" :key="job.id">
              <tr class="hover:bg-ink-800/60">
                <td class="px-4 py-2.5 text-xs text-text-muted">{{ timeAgo(job.created_at) }}</td>
                <td class="px-4 py-2.5">
                  <span class="text-text-primary">{{ job.data_type }}</span>
                  <span v-if="job.dry_run" class="ml-2 rounded bg-ink-700 px-1.5 py-0.5 text-xs text-text-muted">dry-run</span>
                </td>
                <td class="px-4 py-2.5 max-w-[220px] truncate text-xs text-text-secondary" :title="job.source_file">
                  {{ job.source_file || '—' }}
                </td>
                <td class="px-4 py-2.5 text-text-secondary">{{ job.total_count }}</td>
                <td class="px-4 py-2.5 text-text-secondary">{{ job.created_count }} / {{ job.updated_count }}</td>
                <td class="px-4 py-2.5">
                  <span :class="isJobFailed(job) ? 'text-danger-mc' : 'text-text-muted'">{{ job.failed_count }}</span>
                </td>
                <td class="px-4 py-2.5 text-xs text-text-muted">{{ job.operator || '—' }}</td>
                <td class="px-4 py-2.5 text-right">
                  <button
                    v-if="job.error_summary?.length"
                    type="button"
                    class="rounded border border-border-light px-2 py-1 text-xs text-grass-400 hover:bg-ink-700"
                    @click="toggleExpand(job.id)"
                  >
                    {{ expanded === job.id ? '收起' : `错误 (${job.error_summary.length})` }}
                  </button>
                  <span v-else class="text-xs text-text-muted">—</span>
                </td>
              </tr>
              <tr v-if="expanded === job.id" class="bg-ink-900">
                <td colspan="8" class="px-4 py-3">
                  <ul class="space-y-1 text-xs text-text-secondary">
                    <li v-for="(msg, i) in job.error_summary ?? []" :key="i">· {{ msg }}</li>
                  </ul>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>
      <div v-else-if="!jobsLoading" class="card p-10 text-center text-sm text-text-muted">暂无导入任务记录。</div>
    </ListShell>

    <Pagination
      :page="jobFilters.page"
      :page-size="jobFilters.pageSize"
      :total="jobsTotal"
      :total-pages="jobsTotalPages"
      @change="onJobPageChange"
    />
  </div>
</template>
