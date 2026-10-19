<script setup lang="ts">
// 审计日志 + 安全事件（需求 6.4 / gj.md 阶段三）
//
// 两个视图共用同一份 audit_logs：
//   * 审计日志：全部后台操作，支持动作 / 对象类型 / 结果 / 来源 IP 筛选；
//   * 安全事件：只看登录、改密、MFA、会话撤销这类账号安全动作（后端按动作前缀过滤）。
import { onMounted, reactive, ref } from 'vue'
import ListShell from '@/components/admin/list-shell.vue'
import AdminTable from '@/components/admin/AdminTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useUiStore } from '@/stores/ui'
import { exportAuditLogs, listAuditLogs } from '@/api/admin-system'
import type { AdminTableColumn } from '@/types/admin'
import type { AuditLog } from '@/types/audit'
import { ApiError } from '@/types/api'
import { formatDateTime } from '@/utils/format'

const columns: AdminTableColumn[] = [
  { key: 'created_at', label: '时间', width: '170px' },
  { key: 'actor', label: '操作者' },
  { key: 'action', label: '动作' },
  { key: 'object', label: '对象' },
  { key: 'result', label: '结果', width: '90px' },
  { key: 'request_id', label: '请求 ID' },
]

type AuditMode = 'all' | 'security'

const ui = useUiStore()
/** 当前视图：全部审计 / 安全事件 */
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

onMounted(load)
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
        安全事件
      </button>
      <span v-if="mode === 'security'" class="text-xs text-text-muted">
        仅显示登录 / 改密 / MFA / 会话撤销
      </span>
    </div>

    <ListShell title="操作日志" :loading="loading" :error="error" @retry="load">
      <div class="mb-4 flex flex-wrap gap-3">
        <input
          v-model="filters.action"
          type="text"
          placeholder="动作（如 resource.create）"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          @change="filters.page = 1; load()"
        />
        <input
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
      </div>

      <AdminTable
        :columns="columns"
        :rows="items"
        row-key="id"
        min-width="760px"
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
        <template #cell-object="{ row }">
          <span class="text-text-primary">{{ row.object_type }}</span>
          <span v-if="row.object_id" class="ml-1 font-mono text-xs text-text-muted">
            {{ row.object_id.slice(0, 8) }}
          </span>
        </template>
        <template #cell-result="{ row }">
          <span class="text-xs font-medium" :class="resultClass(row.result)">{{ row.result }}</span>
        </template>
        <template #cell-request_id="{ row }">
          <span class="font-mono text-xs text-text-muted">{{ row.request_id || '—' }}</span>
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