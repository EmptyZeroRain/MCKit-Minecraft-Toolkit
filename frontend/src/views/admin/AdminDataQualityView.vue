<script setup lang="ts">
// 数据质量监控（TODO §2 版本化资料规则 / §4 数据导入与质量监控）
// 仅 admin 角色可见（侧边栏过滤 + 后端 403 兜底）。
// 导入任务已拆到「内容健康 → 数据导入任务」独立 Tab，本页只负责质量概览与工单。
import { computed, onMounted, reactive, ref } from 'vue'
import ListShell from '@/components/admin/list-shell.vue'
import { useUiStore } from '@/stores/ui'
import {
  fetchDataQualityOverview,
  listQualityAssignees,
  updateQualityIssue,
} from '@/api/admin-data-quality'
import { ApiError } from '@/types/api'
import type {
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

function reloadAll() {
  loadOverview()
  ui.success('已刷新')
}

// ---- 需要关注的检查项折叠（默认收起：每项都带下拉与输入框，条目多时过长）----
const issuesOpen = ref(false)

function toggleIssues() {
  issuesOpen.value = !issuesOpen.value
}

// ---- 全部检查项的分组折叠（默认全部收起，点击分类标题展开）----
/** 当前展开的分类 key 集合；整体替换以保证 Set 变更可被追踪 */
const openGroups = ref<Set<string>>(new Set())

function isGroupOpen(category: string): boolean {
  return openGroups.value.has(category)
}

function toggleGroup(category: string) {
  const next = new Set(openGroups.value)
  if (next.has(category)) next.delete(category)
  else next.add(category)
  openGroups.value = next
}

/** 是否全部展开（用于「展开全部 / 收起全部」按钮文案与行为） */
const allGroupsOpen = computed(
  () => grouped.value.length > 0 && grouped.value.every((g) => openGroups.value.has(g.category)),
)

function toggleAllGroups() {
  openGroups.value = allGroupsOpen.value ? new Set() : new Set(grouped.value.map((g) => g.category))
}

/** 分类内有命中的检查项数量（收起状态下也能看出该分类是否有问题） */
function groupHitCount(checks: DataQualityCheck[]): number {
  return checks.filter((c) => c.count > 0).length
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
          客观检查资料库的缺失、非法格式与悬空引用；按分类折叠展示全部检查项，命中项可指派负责人并跟进处理状态。
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

        <!-- 需要关注的检查项（含分派 / 处理状态 / 直接跳转）
             该区域每个问题都带负责人下拉与备注输入，条目多时会把下方检查项挤到很远处，
             因此默认收起，仅在标题栏显示汇总计数。 -->
        <div class="mb-5">
          <button
            type="button"
            class="flex w-full items-center justify-between gap-3 rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-left transition hover:bg-ink-800/60"
            :aria-expanded="issuesOpen"
            aria-controls="quality-issues-body"
            @click="toggleIssues"
          >
            <span class="flex min-w-0 flex-wrap items-center gap-2 text-sm font-semibold text-text-secondary">
              <svg
                class="h-4 w-4 shrink-0 text-text-muted transition-transform"
                :class="issuesOpen ? 'rotate-90' : ''"
                viewBox="0 0 20 20"
                fill="currentColor"
                aria-hidden="true"
              >
                <path
                  fill-rule="evenodd"
                  d="M7.21 14.77a.75.75 0 01.02-1.06L11.168 10 7.23 6.29a.75.75 0 111.04-1.08l4.5 4.25a.75.75 0 010 1.08l-4.5 4.25a.75.75 0 01-1.06-.02z"
                  clip-rule="evenodd"
                />
              </svg>
              <span>需要关注的检查项</span>
              <span class="text-xs font-normal text-text-muted">{{ actionable.length }} 项</span>
              <span v-if="overview.error_checks" class="rounded bg-danger-mc/15 px-2 py-0.5 text-xs font-normal text-danger-mc">
                严重 {{ overview.error_checks }}
              </span>
              <span v-if="overview.warning_checks" class="rounded bg-amber-500/15 px-2 py-0.5 text-xs font-normal text-amber-300">
                提醒 {{ overview.warning_checks }}
              </span>
            </span>
            <span class="shrink-0 text-xs text-text-muted">{{ issuesOpen ? '收起' : '展开' }}</span>
          </button>

          <div
            v-if="!actionable.length"
            class="mt-2 rounded-md border border-grass-600/30 bg-grass-600/10 p-3 text-sm text-grass-300"
          >
            未发现需要处理的数据质量问题。
          </div>
          <div v-else-if="issuesOpen" id="quality-issues-body" class="mt-2 space-y-2">
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
        </div>

        <!-- 全部检查项：按分类折叠，默认全部收起，点击标题展开该分类全部检查项 -->
        <div class="space-y-3">
          <div class="flex items-center justify-between gap-3">
            <h2 class="text-sm font-semibold text-text-secondary">
              全部检查项
              <span class="ml-1 font-normal text-text-muted">（{{ grouped.length }} 个分类）</span>
            </h2>
            <button
              type="button"
              class="shrink-0 rounded-md border border-border-dark px-3 py-1.5 text-xs text-text-secondary transition hover:bg-ink-700"
              @click="toggleAllGroups"
            >
              {{ allGroupsOpen ? '收起全部' : '展开全部' }}
            </button>
          </div>

          <div v-for="g in grouped" :key="g.category" class="card overflow-hidden">
            <button
              type="button"
              class="flex w-full items-center justify-between gap-3 px-4 py-3 text-left transition hover:bg-ink-800/60"
              :aria-expanded="isGroupOpen(g.category)"
              :aria-controls="`quality-group-${g.category}`"
              @click="toggleGroup(g.category)"
            >
              <span class="flex min-w-0 flex-wrap items-center gap-2 text-sm font-semibold text-text-secondary">
                <svg
                  class="h-4 w-4 shrink-0 text-text-muted transition-transform"
                  :class="isGroupOpen(g.category) ? 'rotate-90' : ''"
                  viewBox="0 0 20 20"
                  fill="currentColor"
                  aria-hidden="true"
                >
                  <path
                    fill-rule="evenodd"
                    d="M7.21 14.77a.75.75 0 01.02-1.06L11.168 10 7.23 6.29a.75.75 0 111.04-1.08l4.5 4.25a.75.75 0 010 1.08l-4.5 4.25a.75.75 0 01-1.06-.02z"
                    clip-rule="evenodd"
                  />
                </svg>
                <span class="truncate">{{ categoryLabel(g.category) }}</span>
                <span class="text-xs font-normal text-text-muted">{{ g.checks.length }} 项</span>
                <span
                  v-if="groupHitCount(g.checks)"
                  class="rounded bg-amber-500/15 px-2 py-0.5 text-xs font-normal text-amber-300"
                >
                  {{ groupHitCount(g.checks) }} 项有命中
                </span>
              </span>
              <span class="shrink-0 text-xs text-text-muted">
                {{ isGroupOpen(g.category) ? '收起' : '展开' }}
              </span>
            </button>

            <div v-if="isGroupOpen(g.category)" :id="`quality-group-${g.category}`" class="overflow-x-auto">
              <table class="w-full min-w-[640px] text-left text-sm">
                <thead class="border-b border-t border-border-dark bg-ink-900 text-xs text-text-muted">
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
  </div>
</template>
