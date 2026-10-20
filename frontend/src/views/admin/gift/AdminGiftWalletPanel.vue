<script setup lang="ts">
// 礼物钱包面板：查询用户双币余额、受控增减、查看流水。
//
// 所有余额变化必须写 gift_ledger；后台只提供受控增减，不提供直接改余额接口。
// 扣减不能使余额为负；同一 request_key 重复提交只产生一笔流水。
import { onMounted, reactive, ref } from 'vue'
import AdminTable from '@/components/admin/AdminTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import { ApiError } from '@/types/api'
import type { AdminTableColumn } from '@/types/admin'
import { adjustGiftWallet, fetchAdminGiftLedger, fetchAdminGiftWallet } from '@/api/gift'
import type { GiftLedgerItem, GiftWallet } from '@/types/gift'

const inputClass =
  'w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none'

const error = ref('')
const notice = ref('')

const targetUserId = ref('')
const wallet = ref<GiftWallet | null>(null)
// 记录钱包查询时使用的目标值，防止查询 A 后改成 B 直接提交导致误操作。
const walletTarget = ref('')
const walletLoading = ref(false)
const walletError = ref('')
const adjusting = ref(false)
const adjustError = ref('')
const adjustForm = reactive<{ coin_type: 'general' | 'checkin'; amount: number; reason: string }>({
  coin_type: 'general',
  amount: 0,
  reason: '',
})

const loading = ref(false)
const items = ref<GiftLedgerItem[]>([])
const total = ref(0)
const totalPages = ref(1)
const filters = reactive<{
  user_id: string
  coin_type: '' | 'general' | 'checkin'
  event_type: string
  page: number
  pageSize: number
}>({ user_id: '', coin_type: '', event_type: '', page: 1, pageSize: 20 })

const columns: AdminTableColumn[] = [
  { key: 'created_at', label: '时间', width: '170px' },
  { key: 'user_id', label: '用户 ID', width: '240px' },
  { key: 'coin_type', label: '币种', width: '90px' },
  { key: 'amount', label: '变动', width: '90px' },
  { key: 'balance_after', label: '变动后余额', width: '110px' },
  { key: 'event_type', label: '类型', width: '120px' },
  { key: 'reason', label: '原因' },
  { key: 'operator_admin_id', label: '操作人', width: '240px' },
]

function formatTime(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString('zh-CN')
}

function formatAmount(n: number): string {
  return `${n > 0 ? '+' : ''}${n}`
}

function coinLabel(coinType: string): string {
  return coinType === 'checkin' ? '签到礼物币' : '通用礼物币'
}

function eventLabel(eventType: string): string {
  switch (eventType) {
    case 'admin_adjustment':
      return '后台调整'
    case 'daily_checkin':
      return '每日签到'
    case 'gift_redeem':
      return '兑换礼物'
    default:
      return eventType
  }
}

function newRequestKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `gift-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

async function loadWallet() {
  walletError.value = ''
  if (!targetUserId.value.trim()) {
    walletError.value = '请填写目标用户 ID'
    return
  }
  walletLoading.value = true
  try {
    wallet.value = await fetchAdminGiftWallet(targetUserId.value.trim())
    walletTarget.value = targetUserId.value.trim()
  } catch (e) {
    wallet.value = null
    walletTarget.value = ''
    walletError.value = e instanceof ApiError ? e.message : '钱包查询失败'
  } finally {
    walletLoading.value = false
  }
}

async function submitAdjust() {
  adjustError.value = ''
  notice.value = ''
  if (!targetUserId.value.trim()) {
    adjustError.value = '请填写目标用户 ID'
    return
  }
  if (walletTarget.value !== targetUserId.value.trim()) {
    adjustError.value = '请先查询目标用户钱包，确认账号与余额后再调整'
    return
  }
  if (!adjustForm.amount) {
    adjustError.value = '调整值不能为 0'
    return
  }
  if (!adjustForm.reason.trim()) {
    adjustError.value = '请填写调整原因'
    return
  }
  if (
    !window.confirm(
      `确定为该用户${adjustForm.amount > 0 ? '增加' : '扣减'} ${Math.abs(adjustForm.amount)} ${coinLabel(adjustForm.coin_type)}吗？`,
    )
  ) {
    return
  }
  adjusting.value = true
  try {
    const entry = await adjustGiftWallet({
      target_user_id: targetUserId.value.trim(),
      coin_type: adjustForm.coin_type,
      amount: Number(adjustForm.amount),
      reason: adjustForm.reason.trim(),
      request_key: newRequestKey(),
    })
    notice.value = `调整完成，${coinLabel(entry.coin_type)}变动后余额 ${entry.balance_after}。`
    adjustForm.amount = 0
    adjustForm.reason = ''
    await Promise.all([loadWallet(), loadLedger()])
  } catch (e) {
    adjustError.value = e instanceof ApiError ? e.message : '调整失败'
  } finally {
    adjusting.value = false
  }
}

async function loadLedger() {
  loading.value = true
  error.value = ''
  try {
    const res = await fetchAdminGiftLedger({
      user_id: filters.user_id.trim() || undefined,
      coin_type: filters.coin_type || undefined,
      event_type: filters.event_type || undefined,
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
  void loadLedger()
}

function changePage(p: number) {
  filters.page = p
  void loadLedger()
}

onMounted(loadLedger)
</script>

<template>
  <div>
    <p v-if="error" class="mb-3 rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-2 text-sm text-danger-mc" role="alert">{{ error }}</p>
    <p v-if="notice" class="mb-3 rounded-card border border-grass-500/40 bg-grass-500/10 px-4 py-2 text-sm text-grass-400" role="status">{{ notice }}</p>

    <div class="card mb-5 grid gap-3 p-4 sm:grid-cols-4">
      <h2 class="text-sm font-medium text-text-secondary sm:col-span-4">用户钱包</h2>
      <label class="text-xs text-text-muted sm:col-span-3">
        目标用户 ID（内部 UUID 或对外 ID）
        <input v-model="targetUserId" :class="inputClass" type="text" placeholder="UUID / U7K3M2QX9F" @keyup.enter="loadWallet" />
      </label>
      <div class="flex items-end">
        <button
          type="button"
          class="w-full rounded-md border border-border-light bg-ink-700 px-3 py-2 text-sm text-text-secondary transition hover:bg-ink-600 disabled:opacity-50"
          :disabled="walletLoading"
          @click="loadWallet"
        >
          查询钱包
        </button>
      </div>

      <div v-if="wallet" class="rounded-card border border-border-dark bg-ink-900/60 p-4 sm:col-span-4">
        <div class="flex flex-wrap items-baseline gap-x-4 gap-y-1">
          <span class="text-sm text-text-secondary">{{ wallet.username }}</span>
          <span class="font-mono text-xs text-text-muted">{{ wallet.user_no }}</span>
          <span class="ml-auto text-2xl font-bold text-grass-300">{{ wallet.total_balance }}</span>
          <span class="text-xs text-text-muted">礼物币（合计）</span>
        </div>
        <div class="mt-2 flex flex-wrap gap-x-6 gap-y-1 text-xs text-text-secondary">
          <span>通用礼物币：<strong class="text-text-primary">{{ wallet.general_balance }}</strong></span>
          <span>签到礼物币：<strong class="text-text-primary">{{ wallet.checkin_balance }}</strong></span>
        </div>
        <p class="mt-1 text-[11px] text-text-muted">
          最后变动：{{ wallet.updated_at ? formatTime(wallet.updated_at) : '尚无流水' }}
        </p>
      </div>
      <p v-if="walletError" class="text-xs text-danger-mc sm:col-span-4" role="alert">{{ walletError }}</p>
    </div>

    <form class="card mb-5 grid gap-3 p-4 sm:grid-cols-4" @submit.prevent="submitAdjust">
      <h2 class="text-sm font-medium text-text-secondary sm:col-span-4">受控增减礼物币</h2>
      <label class="text-xs text-text-muted">
        币种
        <select v-model="adjustForm.coin_type" :class="inputClass">
          <option value="general">通用礼物币</option>
          <option value="checkin">签到礼物币</option>
        </select>
      </label>
      <label class="text-xs text-text-muted">
        变动值（正数增加，负数扣减）
        <input v-model.number="adjustForm.amount" :class="inputClass" type="number" />
      </label>
      <label class="text-xs text-text-muted sm:col-span-2">
        原因（必填）
        <input v-model="adjustForm.reason" :class="inputClass" type="text" maxlength="500" placeholder="如 活动奖励 / 误发扣回" />
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

    <div class="card mb-4 grid gap-3 p-4 sm:grid-cols-5">
      <label class="text-xs text-text-muted sm:col-span-2">
        用户 ID（可留空，支持对外 ID）
        <input v-model="filters.user_id" :class="inputClass" type="text" placeholder="UUID / U7K3M2QX9F" />
      </label>
      <label class="text-xs text-text-muted">
        币种
        <select v-model="filters.coin_type" :class="inputClass">
          <option value="">全部</option>
          <option value="general">通用礼物币</option>
          <option value="checkin">签到礼物币</option>
        </select>
      </label>
      <label class="text-xs text-text-muted">
        事件类型
        <select v-model="filters.event_type" :class="inputClass">
          <option value="">全部</option>
          <option value="admin_adjustment">后台调整</option>
          <option value="daily_checkin">每日签到</option>
          <option value="gift_redeem">兑换礼物</option>
        </select>
      </label>
      <div class="flex items-end">
        <button
          type="button"
          class="w-full rounded-md bg-grass-600 px-3 py-2 text-sm font-medium text-white transition hover:bg-grass-500"
          @click="search"
        >
          查询流水
        </button>
      </div>
    </div>

    <AdminTable :columns="columns" :rows="items" :loading="loading" row-key="id" empty-text="暂无礼物币流水">
      <template #cell-created_at="{ row }">{{ formatTime(row.created_at) }}</template>
      <template #cell-user_id="{ row }">
        <span class="font-mono text-xs">{{ row.user_id }}</span>
      </template>
      <template #cell-coin_type="{ row }">
        <span class="text-xs">{{ coinLabel(row.coin_type) }}</span>
      </template>
      <template #cell-amount="{ row }">
        <span :class="row.amount > 0 ? 'text-grass-300' : 'text-danger-mc'">{{ formatAmount(row.amount) }}</span>
      </template>
      <template #cell-event_type="{ row }">
        <span class="text-xs">{{ eventLabel(row.event_type) }}</span>
      </template>
      <template #cell-operator_admin_id="{ row }">
        <span class="font-mono text-xs text-text-muted">{{ row.operator_admin_id || '—' }}</span>
      </template>
    </AdminTable>

    <Pagination
      class="mt-4"
      :page="filters.page"
      :page-size="filters.pageSize"
      :total="total"
      :total-pages="totalPages"
      @change="changePage"
    />
  </div>
</template>
