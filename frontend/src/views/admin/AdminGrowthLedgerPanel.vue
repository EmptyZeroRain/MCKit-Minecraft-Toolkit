<script setup lang="ts">
// 经验流水与人工调整 / 冲正（jingyan.md §10.5）。
//
// 账本只增不删：冲正以等额负向流水表达，原始流水永久保留。
// 人工调整与冲正均需填写原因，属高风险操作（后端限 admin）。
import { onMounted, reactive, ref } from 'vue'
import AdminTable from '@/components/admin/AdminTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import { ApiError } from '@/types/api'
import type { AdminTableColumn } from '@/types/admin'
import { adjustUserXP, fetchAdminGrowthLedger, reverseGrowthLedger } from '@/api/growth'
import { LEDGER_SOURCE_LABELS, type GrowthLedgerItem } from '@/types/growth'

const loading = ref(false)
const error = ref('')
const notice = ref('')
const items = ref<GrowthLedgerItem[]>([])
const total = ref(0)
const totalPages = ref(1)

const filters = reactive<{ user_id: string; rule_code: string; source_type: string; page: number; pageSize: number }>(
  { user_id: '', rule_code: '', source_type: '', page: 1, pageSize: 20 },
)

const sourceOptions = Object.keys(LEDGER_SOURCE_LABELS)

const inputClass =
  'w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none'

// 人工调整
const adjust = reactive({ target_user_id: '', delta_xp: 0, reason: '' })
const adjusting = ref(false)
const adjustError = ref('')

// 冲正
const reversing = ref<GrowthLedgerItem | null>(null)
const reverseReason = ref('')
const reverseError = ref('')
const reverseBusy = ref(false)

const columns: AdminTableColumn[] = [
  { key: 'created_at', label: '时间', width: '170px' },
  { key: 'user_id', label: '用户 ID', width: '240px' },
  { key: 'delta_xp', label: '变动', width: '90px' },
  { key: 'balance_after', label: '余额', width: '90px' },
  { key: 'source_type', label: '来源', width: '100px' },
  { key: 'rule_code', label: '规则' },
  { key: 'reason', label: '说明' },
  { key: 'action', label: '操作', align: 'right', width: '90px' },
]

function sourceLabel(t: string): string {
  return LEDGER_SOURCE_LABELS[t] ?? t
}

function formatTime(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString('zh-CN')
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await fetchAdminGrowthLedger({
      user_id: filters.user_id.trim() || undefined,
      rule_code: filters.rule_code.trim() || undefined,
      source_type: filters.source_type || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    })
    items.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '流水加载失败'
  } finally {
    loading.value = false
  }
}

function search() {
  filters.page = 1
  void load()
}

function changePage(p: number) {
  filters.page = p
  void load()
}

onMounted(load)

async function submitAdjust() {
  adjustError.value = ''
  notice.value = ''
  if (!adjust.target_user_id.trim()) {
    adjustError.value = '请填写目标用户 ID'
    return
  }
  if (!adjust.reason.trim()) {
    adjustError.value = '请填写调整原因'
    return
  }
  if (!adjust.delta_xp) {
    adjustError.value = '调整值不能为 0'
    return
  }
  adjusting.value = true
  try {
    await adjustUserXP({
      target_user_id: adjust.target_user_id.trim(),
      delta_xp: Number(adjust.delta_xp),
      reason: adjust.reason.trim(),
    })
    notice.value = '人工调整已完成并写入账本与审计。'
    adjust.target_user_id = ''
    adjust.delta_xp = 0
    adjust.reason = ''
    await load()
  } catch (e) {
    adjustError.value = e instanceof ApiError ? e.message : '调整失败'
  } finally {
    adjusting.value = false
  }
}

function startReverse(item: GrowthLedgerItem) {
  reversing.value = item
  reverseReason.value = ''
  reverseError.value = ''
}

async function submitReverse() {
  if (!reversing.value) return
  reverseError.value = ''
  notice.value = ''
  if (!reverseReason.value.trim()) {
    reverseError.value = '请填写冲正原因'
    return
  }
  reverseBusy.value = true
  try {
    await reverseGrowthLedger(reversing.value.id, reverseReason.value.trim())
    notice.value = '已生成冲正流水，原始流水保留。'
    reversing.value = null
    await load()
  } catch (e) {
    reverseError.value = e instanceof ApiError ? e.message : '冲正失败'
  } finally {
    reverseBusy.value = false
  }
}
</script>

<template>
  <div>
    <!-- 人工调整 -->
    <form class="card mb-5 grid gap-3 p-4 sm:grid-cols-4" @submit.prevent="submitAdjust">
      <h2 class="text-sm font-medium text-text-secondary sm:col-span-4">人工调整经验</h2>
      <label class="text-xs text-text-muted sm:col-span-2">
        目标用户 ID
        <input v-model="adjust.target_user_id" :class="inputClass" type="text" placeholder="UUID" />
      </label>
      <label class="text-xs text-text-muted">
        调整值（可负）
        <input v-model.number="adjust.delta_xp" :class="inputClass" type="number" />
      </label>
      <label class="text-xs text-text-muted">
        原因（必填）
        <input v-model="adjust.reason" :class="inputClass" type="text" placeholder="如 活动补发" />
      </label>
      <p v-if="adjustError" class="text-xs text-danger-mc sm:col-span-4" role="alert">{{ adjustError }}</p>
      <button
        type="submit"
        class="rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-60 sm:col-span-4 sm:w-auto sm:justify-self-start"
        :disabled="adjusting"
      >
        提交调整
      </button>
    </form>

    <!-- 冲正 -->
    <div v-if="reversing" class="card mb-5 border-danger-mc/40 p-4">
      <h2 class="mb-2 text-sm font-medium text-danger-mc">
        冲正流水 {{ reversing.id.slice(0, 8) }}…（{{ reversing.delta_xp }} 经验）
      </h2>
      <p class="mb-3 text-xs text-text-muted">将生成等额负向流水，原始流水保持不变。</p>
      <label class="mb-3 block text-xs text-text-muted">
        冲正原因（必填）
        <input v-model="reverseReason" :class="inputClass" type="text" placeholder="如 误发奖励" />
      </label>
      <p v-if="reverseError" class="mb-3 text-xs text-danger-mc" role="alert">{{ reverseError }}</p>
      <div class="flex gap-2">
        <button type="button" class="rounded-md bg-danger-mc px-4 py-2 text-sm font-medium text-white transition hover:bg-red-500 disabled:opacity-60" :disabled="reverseBusy" @click="submitReverse">
          确认冲正
        </button>
        <button type="button" class="rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-secondary transition hover:bg-ink-600" @click="reversing = null">
          取消
        </button>
      </div>
    </div>

    <!-- 筛选 -->
    <form class="mb-4 grid gap-3 sm:grid-cols-4" @submit.prevent="search">
      <input v-model="filters.user_id" :class="inputClass" type="text" placeholder="用户 ID" />
      <input v-model="filters.rule_code" :class="inputClass" type="text" placeholder="规则编码" />
      <select v-model="filters.source_type" :class="inputClass">
        <option value="">全部来源</option>
        <option v-for="s in sourceOptions" :key="s" :value="s">{{ sourceLabel(s) }}</option>
      </select>
      <button type="submit" class="rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-secondary transition hover:bg-ink-600">
        查询
      </button>
    </form>

    <p v-if="error" class="mb-3 rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-2 text-sm text-danger-mc" role="alert">{{ error }}</p>
    <p v-if="notice" class="mb-3 rounded-card border border-grass-500/40 bg-grass-500/10 px-4 py-2 text-sm text-grass-400" role="status">{{ notice }}</p>

    <AdminTable :columns="columns" :rows="items" :loading="loading" row-key="id" empty-text="暂无经验流水">
      <template #cell-created_at="{ row }">{{ formatTime(row.created_at) }}</template>
      <template #cell-user_id="{ row }">
        <span class="font-mono text-xs">{{ row.user_id }}</span>
      </template>
      <template #cell-delta_xp="{ row }">
        <span :class="row.delta_xp >= 0 ? 'text-grass-300' : 'text-danger-mc'">
          {{ row.delta_xp >= 0 ? '+' : '' }}{{ row.delta_xp }}
        </span>
      </template>
      <template #cell-source_type="{ row }">{{ sourceLabel(row.source_type) }}</template>
      <template #cell-rule_code="{ row }">
        <span class="font-mono text-xs text-text-muted">{{ row.rule_code || '—' }}</span>
      </template>
      <template #cell-action="{ row }">
        <button
          v-if="row.source_type !== 'reversal' && row.delta_xp !== 0"
          type="button"
          class="rounded-md border border-danger-mc/40 px-2.5 py-1 text-xs text-danger-mc transition hover:bg-danger-mc/10"
          @click="startReverse(row)"
        >
          冲正
        </button>
      </template>
    </AdminTable>

    <div class="mt-4">
      <Pagination :page="filters.page" :page-size="filters.pageSize" :total="total" :total-pages="totalPages" @change="changePage" />
    </div>
  </div>
</template>
