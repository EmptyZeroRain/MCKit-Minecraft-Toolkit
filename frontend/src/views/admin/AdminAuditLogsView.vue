<script setup lang="ts">
// 安全审计：审计日志 + 安全日志（需求 6.4 / gj.md 阶段三）
//
// 两者共用 audit_logs，但定位不同：
//   * 审计日志：全部后台操作的完整留痕（谁、做了什么、对什么、结果、来源 IP），
//     点「详情」可查看数据库里的原始日志（变更前后摘要 + 请求 ID）；
//   * 安全日志：只保留账号安全相关动作（登录 / 改密 / MFA / 会话撤销），
//     用「事件」而不是「动作」呈现，并额外展示登录设备。
//
// 后端通过 security 参数按动作前缀过滤（audit.SecurityActionPrefixes）。
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import ListShell from '@/components/admin/list-shell.vue'
import AdminTable from '@/components/admin/AdminTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useUiStore } from '@/stores/ui'
import { exportAuditLogs, listAuditLogs } from '@/api/admin-system'
import type { AdminTableColumn } from '@/types/admin'
import type { AuditLog } from '@/types/audit'
import { ApiError } from '@/types/api'
import { formatDateTime } from '@/utils/format'
import { describeUserAgent } from '@/utils/user-agent'

type AuditMode = 'all' | 'security'

const ui = useUiStore()
/** 当前视图：全部审计 / 安全日志 */
const mode = ref<AuditMode>('all')

const filters = reactive<{
  action: string
  objectType: string
  result: 'success' | 'failure' | ''
  sourceIp: string
  page: number
  pageSize: number
}>({ action: '', objectType: '', result: '', sourceIp: '', page: 1, pageSize: 20 })

const items = ref<AuditLog[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')
const exporting = ref(false)

/** 安全事件的动作 → 中文名（未收录的动作直接显示原始 action）。 */
const SECURITY_EVENT_LABELS: Record<string, string> = {
  'admin.login': '登录',
  'admin.login.mfa': 'MFA 登录校验',
  'admin.login.mfa_challenge': '登录（待第二因子）',
  'admin.password.change': '修改密码',
  'admin.mfa.setup': 'MFA 绑定初始化',
  'admin.mfa.enable': 'MFA 启用',
  'admin.mfa.disable': 'MFA 解绑',
  'admin.revoke_sessions': '撤销本人会话',
  'session.admin_revoke': '管理员强制下线',
}

/** 安全日志的动作下拉：与后端安全前缀保持一致，避免筛出空结果。 */
const SECURITY_ACTION_OPTIONS = [
  { value: '', label: '全部安全事件' },
  { value: 'admin.login', label: '登录' },
  { value: 'admin.login.mfa', label: 'MFA 登录校验' },
  { value: 'admin.password.change', label: '修改密码' },
  { value: 'admin.mfa.setup', label: 'MFA 绑定初始化' },
  { value: 'admin.mfa.enable', label: 'MFA 启用' },
  { value: 'admin.mfa.disable', label: 'MFA 解绑' },
  { value: 'admin.revoke_sessions', label: '撤销本人会话' },
  { value: 'session.admin_revoke', label: '管理员强制下线' },
]

/**
 * 列定义随视图切换：
 *   - 审计日志看「动作 + 对象」；
 *   - 安全日志看「事件 + 账号 + 设备」，登录失败时 IP / 设备比对象更重要。
 */
const columns = computed<AdminTableColumn[]>(() =>
  mode.value === 'security'
    ? [
        { key: 'created_at', label: '时间', width: '170px' },
        { key: 'event', label: '安全事件' },
        { key: 'actor', label: '账号' },
        { key: 'source_ip', label: '来源 IP', width: '140px' },
        { key: 'device', label: '设备' },
        { key: 'result', label: '结果', width: '80px' },
        { key: 'detail', label: '详情', align: 'right', width: '80px' },
      ]
    : [
        { key: 'created_at', label: '时间', width: '170px' },
        { key: 'actor', label: '操作者' },
        { key: 'action', label: '动作' },
        { key: 'object', label: '对象' },
        { key: 'source_ip', label: '来源 IP', width: '140px' },
        { key: 'result', label: '结果', width: '80px' },
        { key: 'detail', label: '详情', align: 'right', width: '80px' },
      ],
)

/** 当前展开原始日志的行。 */
const active = ref<AuditLog | null>(null)

/** 原始日志：按数据库字段原样输出，不做字段裁剪或翻译。 */
const rawLog = computed(() => {
  const r = active.value
  if (!r) return ''
  return JSON.stringify(
    {
      id: r.id,
      created_at: r.created_at,
      action: r.action,
      result: r.result,
      actor_admin_id: r.actor_admin_id,
      actor_username: r.actor_username ?? null,
      object_type: r.object_type,
      object_id: r.object_id,
      source_ip: r.source_ip,
      request_id: r.request_id,
      before_summary: r.before_summary,
      after_summary: r.after_summary,
    },
    null,
    2,
  )
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await listAuditLogs({
      action: filters.action || undefined,
      object_type: filters.objectType || undefined,
      result: filters.result || undefined,
      source_ip: filters.sourceIp || undefined,
      security: mode.value === 'security' || undefined,
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

function onPageChange(p: number) {
  filters.page = p
  load()
}

function switchMode(next: AuditMode) {
  if (mode.value === next) return
  mode.value = next
  filters.page = 1
  // 两个视图的动作取值域不同：审计用自由文本，安全日志用固定枚举。
  // 不清理会把上一个视图的条件带过去，出现「明明有日志却全部空白」。
  filters.action = ''
  filters.objectType = ''
  active.value = null
  load()
}

/** 当前筛选条件导出 CSV（最多 5000 条，超出会在响应头标记）。 */
async function handleExport() {
  if (exporting.value) return
  exporting.value = true
  try {
    const { blob, truncated } = await exportAuditLogs({
      action: filters.action || undefined,
      object_type: filters.objectType || undefined,
      result: filters.result || undefined,
      source_ip: filters.sourceIp || undefined,
      security: mode.value === 'security' || undefined,
    })
    triggerDownload(blob)
    ui.success(truncated ? '已导出（超出上限，仅含前 5000 条）' : '已导出 CSV')
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '导出失败')
  } finally {
    exporting.value = false
  }
}

function triggerDownload(blob: Blob) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `audit-logs-${new Date().toISOString().slice(0, 10)}.csv`
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

function resultClass(result: string): string {
  return result === 'success' ? 'text-grass-300' : 'text-danger-mc'
}

function resultLabel(result: string): string {
  return result === 'success' ? '成功' : '失败'
}

function eventLabel(action: string): string {
  return SECURITY_EVENT_LABELS[action] ?? action
}

/** 登录设备取自审计 after_summary.user_agent（历史记录可能没有）。 */
function deviceOf(row: AuditLog): string {
  const ua = row.after_summary?.user_agent
  return describeUserAgent(typeof ua === 'string' ? ua : null)
}

async function copyRaw() {
  try {
    await navigator.clipboard.writeText(rawLog.value)
    ui.success('原始日志已复制')
  } catch {
    ui.warning('复制失败，请手动选择文本复制')
  }
}

/** 关闭原始日志抽屉。 */
function closeDetail() {
  active.value = null
}

/** Esc 关闭抽屉（与 ConfirmDialog 的交互保持一致）。 */
function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && active.value) closeDetail()
}

onMounted(() => {
  load()
  window.addEventListener('keydown', onKeydown)
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-wrap items-center gap-2">
      <button
        type="button"
        class="rounded-md px-3 py-1.5 text-sm transition"
        :class="mode === 'all' ? 'bg-grass-500/15 text-grass-300' : 'text-text-secondary hover:bg-ink-700'"
        @click="switchMode('all')"
      >
        审计日志
      </button>
      <button
        type="button"
        class="rounded-md px-3 py-1.5 text-sm transition"
        :class="mode === 'security' ? 'bg-grass-500/15 text-grass-300' : 'text-text-secondary hover:bg-ink-700'"
        @click="switchMode('security')"
      >
        安全日志
      </button>
    </div>

    <p class="text-xs text-text-muted">
      <template v-if="mode === 'all'">
        审计日志记录全部后台操作；点「详情」可查看原始日志（变更前后摘要与请求 ID）。
      </template>
      <template v-else>
        安全日志只包含账号安全动作：登录 / 改密 / MFA / 会话撤销，含登录设备的原始
        User-Agent 归纳结果。
      </template>
    </p>

    <ListShell title="操作日志" :loading="loading" :error="error" @retry="load">
      <div class="mb-4 flex flex-wrap gap-3">
        <!-- 审计日志用自由文本查动作；安全日志用固定枚举，避免输入不存在的动作 -->
        <select
          v-if="mode === 'security'"
          v-model="filters.action"
          aria-label="安全事件类型"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="filters.page = 1; load()"
        >
          <option v-for="opt in SECURITY_ACTION_OPTIONS" :key="opt.value" :value="opt.value">
            {{ opt.label }}
          </option>
        </select>
        <input
          v-else
          v-model="filters.action"
          type="text"
          placeholder="动作（如 resource.create）"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          @change="filters.page = 1; load()"
        />
        <input
          v-if="mode === 'all'"
          v-model="filters.objectType"
          type="text"
          placeholder="对象类型"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          @change="filters.page = 1; load()"
        />
        <input
          v-model="filters.sourceIp"
          type="text"
          placeholder="来源 IP"
          class="h-9 w-40 rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          @change="filters.page = 1; load()"
        />
        <select
          v-model="filters.result"
          aria-label="结果"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="filters.page = 1; load()"
        >
          <option value="">全部结果</option>
          <option value="success">成功</option>
          <option value="failure">失败</option>
        </select>
        <button
          type="button"
          class="h-9 rounded-md border border-border-dark px-3 text-sm text-text-secondary transition hover:bg-ink-700 disabled:opacity-50"
          :disabled="exporting || loading"
          @click="handleExport"
        >
          {{ exporting ? '导出中…' : '导出 CSV' }}
        </button>
        <span class="self-center text-xs text-text-muted">
          共 {{ total }} 条
        </span>
      </div>

      <AdminTable
        :columns="columns"
        :rows="items"
        row-key="id"
        min-width="860px"
        empty-text="暂无日志。"
      >
        <template #cell-created_at="{ row }">
          <span class="whitespace-nowrap text-xs text-text-secondary">
            {{ formatDateTime(row.created_at) }}
          </span>
        </template>
        <template #cell-actor="{ row }">
          {{ row.actor_username || row.actor_admin_id || '—' }}
        </template>
        <template #cell-action="{ row }">
          <span class="font-mono text-xs text-grass-300">{{ row.action }}</span>
        </template>
        <template #cell-event="{ row }">
          <span
            class="rounded bg-ink-700 px-2 py-0.5 text-xs"
            :class="row.result === 'success' ? 'text-text-primary' : 'text-danger-mc'"
            :title="row.action"
          >
            {{ eventLabel(row.action) }}
          </span>
        </template>
        <template #cell-object="{ row }">
          <span class="text-text-primary">{{ row.object_type }}</span>
          <span v-if="row.object_id" class="ml-1 font-mono text-xs text-text-muted">
            {{ row.object_id.slice(0, 8) }}
          </span>
        </template>
        <template #cell-source_ip="{ row }">
          <span class="font-mono text-xs text-text-secondary">{{ row.source_ip || '—' }}</span>
        </template>
        <template #cell-device="{ row }">
          <span
            class="text-xs text-text-secondary"
            :title="typeof row.after_summary?.user_agent === 'string' ? row.after_summary.user_agent : '无 User-Agent'"
          >
            {{ deviceOf(row) }}
          </span>
        </template>
        <template #cell-result="{ row }">
          <span class="text-xs font-medium" :class="resultClass(row.result)">
            {{ resultLabel(row.result) }}
          </span>
        </template>
        <template #cell-detail="{ row }">
          <button
            type="button"
            class="rounded border border-border-dark px-2 py-1 text-xs text-text-secondary transition hover:bg-ink-700 hover:text-text-primary"
            @click="active = row"
          >
            查看
          </button>
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

    <!-- 原始日志抽屉：不改写任何字段，便于与数据库 / 导出 CSV 交叉核对 -->
    <Teleport to="body">
      <div
        v-if="active"
        class="fixed inset-0 z-50 flex justify-end bg-black/50"
        role="dialog"
        aria-modal="true"
        aria-label="原始日志"
        @click.self="closeDetail"
      >
        <aside class="h-full w-full max-w-2xl overflow-y-auto bg-ink-900 p-6 shadow-xl">
          <div class="mb-4 flex items-start justify-between gap-4">
            <div>
              <h2 class="text-lg font-semibold text-text-primary">原始日志</h2>
              <p class="mt-1 text-xs text-text-muted">
                审计记录只可读不可改，以下为数据库中的原始内容。
              </p>
            </div>
            <div class="flex shrink-0 gap-2">
              <button
                type="button"
                class="rounded-md border border-border-dark px-3 py-1.5 text-xs text-text-secondary transition hover:bg-ink-700 hover:text-text-primary"
                @click="copyRaw"
              >
                复制
              </button>
              <button
                type="button"
                class="rounded-md border border-border-dark px-3 py-1.5 text-xs text-text-secondary transition hover:bg-ink-700 hover:text-text-primary"
                @click="closeDetail"
              >
                关闭
              </button>
            </div>
          </div>
          <pre
            class="whitespace-pre-wrap break-all rounded-lg bg-ink-950 p-4 font-mono text-xs leading-relaxed text-text-secondary"
          >{{ rawLog }}</pre>
        </aside>
      </div>
    </Teleport>
  </div>
</template>
