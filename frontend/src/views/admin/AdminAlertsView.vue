<script setup lang="ts">
// 运营告警中心（gj.md 阶段三：运营告警及告警历史）
//
// 告警由规则定时评估（默认 5 分钟）产生，也可手动「立即评估」。
// 同一规则未关闭时只保留一条告警；关闭后作为历史保留，不删除。
import { onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import ListShell from '@/components/admin/list-shell.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useUiStore } from '@/stores/ui'
import {
  ackAlert,
  evaluateAlerts,
  fetchAlertSummary,
  listAlerts,
  resolveAlert,
} from '@/api/admin-alerts'
import { ApiError } from '@/types/api'
import {
  ALERT_BADGE_STATUS,
  ALERT_SEVERITY_LABELS,
  ALERT_STATUS_LABELS,
  type AlertSeverity,
  type AlertStatus,
  type AlertSummary,
  type OpsAlert,
} from '@/types/alert'
import { formatDateTime, timeAgo } from '@/utils/format'

const ui = useUiStore()

const filters = reactive<{
  status: AlertStatus | ''
  severity: AlertSeverity | ''
  page: number
  pageSize: number
}>({
  status: 'open',
  severity: '',
  page: 1,
  pageSize: 20,
})

const items = ref<OpsAlert[]>([])
const summary = ref<AlertSummary>({ open: 0, acked: 0, error: 0, warning: 0 })
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')
const evaluating = ref(false)
/** 正在处理（确认 / 关闭）的告警 ID */
const busyId = ref('')
/** 每条告警的关闭原因草稿 */
const notes = reactive<Record<string, string>>({})

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [res, sum] = await Promise.all([
      listAlerts({
        status: filters.status || undefined,
        severity: filters.severity || undefined,
        page: filters.page,
        page_size: filters.pageSize,
      }),
      fetchAlertSummary(),
    ])
    items.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
    summary.value = sum
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function evaluate() {
  if (evaluating.value) return
  evaluating.value = true
  try {
    const sum = await evaluateAlerts()
    const parts = [`评估 ${sum.evaluated} 条规则`, `新增 ${sum.triggered}`, `刷新 ${sum.refreshed}`, `自动关闭 ${sum.auto_closed}`]
    if (sum.errors?.length) {
      ui.warning(`${parts.join(' · ')}；${sum.errors.length} 条规则评估失败`)
    } else {
      ui.success(parts.join(' · '))
    }
    await load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '评估失败')
  } finally {
    evaluating.value = false
  }
}

async function ack(item: OpsAlert) {
  if (busyId.value) return
  busyId.value = item.id
  try {
    await ackAlert(item.id)
    ui.success('已确认，进入处理中')
    await load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '确认失败')
  } finally {
    busyId.value = ''
  }
}

async function resolve(item: OpsAlert) {
  if (busyId.value) return
  busyId.value = item.id
  try {
    await resolveAlert(item.id, notes[item.id] ?? '')
    ui.success('告警已关闭')
    notes[item.id] = ''
    await load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '关闭失败')
  } finally {
    busyId.value = ''
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

let timer: ReturnType<typeof setInterval> | undefined

onMounted(async () => {
  await load()
  // 告警是后台生成的：停留在本页时定时刷新，避免看到过期状态。
  timer = setInterval(() => {
    if (!loading.value) load()
  }, 60_000)
})

onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div class="space-y-5">
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 class="text-lg font-semibold text-text-primary">运营告警</h1>
        <p class="mt-1 text-sm text-text-muted">
          规则每 5 分钟自动评估一次；同一规则在未关闭期间只保留一条告警，关闭后作为历史保留。
        </p>
      </div>
      <button
        type="button"
        class="shrink-0 rounded-md border border-grass-500/50 bg-grass-500/10 px-4 py-2 text-sm text-grass-300 transition hover:bg-grass-500/20 disabled:opacity-50"
        :disabled="evaluating || loading"
        @click="evaluate"
      >
        {{ evaluating ? '评估中…' : '立即评估' }}
      </button>
    </div>

    <div class="grid gap-4 sm:grid-cols-4">
      <div class="card p-4">
        <div class="text-xs text-text-muted">待处理</div>
        <div class="mt-1 text-2xl font-semibold" :class="summary.open ? 'text-amber-300' : 'text-grass-300'">
          {{ summary.open }}
        </div>
      </div>
      <div class="card p-4">
        <div class="text-xs text-text-muted">处理中（已确认）</div>
        <div class="mt-1 text-2xl font-semibold text-blue-300">{{ summary.acked }}</div>
      </div>
      <div class="card p-4">
        <div class="text-xs text-text-muted">未关闭 · 严重</div>
        <div class="mt-1 text-2xl font-semibold" :class="summary.error ? 'text-danger-mc' : 'text-text-secondary'">
          {{ summary.error }}
        </div>
      </div>
      <div class="card p-4">
        <div class="text-xs text-text-muted">未关闭 · 提醒</div>
        <div class="mt-1 text-2xl font-semibold text-amber-300">{{ summary.warning }}</div>
      </div>
    </div>

    <div class="card p-4">
      <div class="flex flex-wrap items-end gap-3">
        <select
          v-model="filters.status"
          aria-label="按状态筛选"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="onFilter"
        >
          <option value="">全部状态</option>
          <option v-for="(label, key) in ALERT_STATUS_LABELS" :key="key" :value="key">{{ label }}</option>
        </select>
        <select
          v-model="filters.severity"
          aria-label="按级别筛选"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="onFilter"
        >
          <option value="">全部级别</option>
          <option value="error">{{ ALERT_SEVERITY_LABELS.error }}</option>
          <option value="warning">{{ ALERT_SEVERITY_LABELS.warning }}</option>
        </select>
        <button
          type="button"
          class="h-9 rounded-md border border-border-dark px-3 text-sm text-text-secondary transition hover:bg-ink-700 disabled:opacity-50"
          :disabled="loading"
          @click="load"
        >
          刷新
        </button>
      </div>
    </div>

    <ListShell title="告警列表" :loading="loading" :error="error" @retry="load">
      <div class="card overflow-hidden">
        <div class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
              <tr>
                <th scope="col" class="px-4 py-3 font-medium">告警</th>
                <th scope="col" class="px-4 py-3 font-medium">级别</th>
                <th scope="col" class="px-4 py-3 font-medium">状态</th>
                <th scope="col" class="px-4 py-3 font-medium">时间</th>
                <th scope="col" class="px-4 py-3 font-medium">处理</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border-dark">
              <tr v-for="item in items" :key="item.id" class="align-top hover:bg-ink-800/60">
                <td class="px-4 py-3">
                  <div class="font-medium text-text-primary">{{ item.title }}</div>
                  <p class="mt-0.5 max-w-xl text-xs text-text-secondary">{{ item.detail }}</p>
                  <div class="mt-1 font-mono text-xs text-text-muted">{{ item.rule_key }}</div>
                </td>
                <td class="px-4 py-3">
                  <span
                    class="rounded px-2 py-0.5 text-xs"
                    :class="
                      item.severity === 'error'
                        ? 'bg-danger-mc/15 text-danger-mc'
                        : 'bg-amber-500/15 text-amber-300'
                    "
                  >
                    {{ ALERT_SEVERITY_LABELS[item.severity] }}
                  </span>
                </td>
                <td class="px-4 py-3">
                  <StatusBadge
                    :status="ALERT_BADGE_STATUS[item.status]"
                    :label="ALERT_STATUS_LABELS[item.status]"
                  />
                  <div v-if="item.resolved_at" class="mt-1 text-xs text-text-muted">
                    {{ item.resolved_by || '系统' }} · {{ formatDateTime(item.resolved_at) }}
                  </div>
                  <div v-else-if="item.acked_at" class="mt-1 text-xs text-text-muted">
                    {{ item.acked_by }} · {{ formatDateTime(item.acked_at) }}
                  </div>
                </td>
                <td class="px-4 py-3 text-xs text-text-muted">
                  <div>触发：{{ timeAgo(item.triggered_at) }}</div>
                  <div class="mt-0.5">最近命中：{{ timeAgo(item.last_seen_at) }}</div>
                </td>
                <td class="px-4 py-3">
                  <template v-if="item.status !== 'resolved'">
                    <input
                      v-model="notes[item.id]"
                      type="text"
                      placeholder="关闭原因（可选）"
                      class="mb-2 w-full rounded-md border border-border-dark bg-ink-900 px-2 py-1 text-xs text-text-primary placeholder:text-text-muted"
                    />
                    <div class="flex flex-wrap gap-2">
                      <button
                        v-if="item.status === 'open'"
                        type="button"
                        class="rounded border border-blue-400/50 bg-blue-400/10 px-2 py-1 text-xs text-blue-300 transition hover:bg-blue-400/20 disabled:opacity-50"
                        :disabled="busyId === item.id"
                        @click="ack(item)"
                      >
                        确认
                      </button>
                      <button
                        type="button"
                        class="rounded border border-grass-500/50 bg-grass-500/10 px-2 py-1 text-xs text-grass-300 transition hover:bg-grass-500/20 disabled:opacity-50"
                        :disabled="busyId === item.id"
                        @click="resolve(item)"
                      >
                        关闭
                      </button>
                    </div>
                  </template>
                  <span v-else class="text-xs text-text-muted">已归档</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-if="!items.length" class="px-4 py-8 text-center text-sm text-text-muted">
          当前筛选下没有告警。
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
