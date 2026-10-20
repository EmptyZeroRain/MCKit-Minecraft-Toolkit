<script setup lang="ts">
// 签到排行榜：按累计签到天数排名普通用户。
import { onMounted, ref } from 'vue'
import AdminTable from '@/components/admin/AdminTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import { ApiError } from '@/types/api'
import type { AdminTableColumn } from '@/types/admin'
import { fetchAdminCheckinRanking } from '@/api/gift'
import type { GiftCheckinRankingItem } from '@/types/gift'

const inputClass =
  'rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none'

const loading = ref(false)
const error = ref('')
const items = ref<GiftCheckinRankingItem[]>([])
const total = ref(0)
const totalPages = ref(1)
const page = ref(1)
const pageSize = 20
const days = ref(0)

const columns: AdminTableColumn[] = [
  { key: 'rank', label: '排名', width: '70px', align: 'center' },
  { key: 'user', label: '用户' },
  { key: 'user_no', label: '对外 ID', width: '120px' },
  { key: 'checkin_days', label: '签到天数', width: '100px', align: 'right' },
  { key: 'checkin_reward', label: '累计签到币', width: '120px', align: 'right' },
  { key: 'last_checkin_date', label: '最近签到', width: '110px' },
]

function displayName(row: GiftCheckinRankingItem): string {
  return row.nickname || row.username
}

function rankClass(rank: number): string {
  if (rank === 1) return 'text-yellow-300'
  if (rank === 2) return 'text-slate-300'
  if (rank === 3) return 'text-amber-500'
  return 'text-text-muted'
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await fetchAdminCheckinRanking({
      days: days.value,
      page: page.value,
      page_size: pageSize,
    })
    items.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '签到排行榜加载失败'
  } finally {
    loading.value = false
  }
}

function search() {
  page.value = 1
  void load()
}

function changePage(p: number) {
  page.value = p
  void load()
}

onMounted(load)
</script>

<template>
  <div>
    <div class="card mb-4 flex flex-wrap items-end gap-3 p-4">
      <label class="text-xs text-text-muted">
        统计范围
        <select v-model.number="days" :class="inputClass" @change="search">
          <option :value="0">全部时间</option>
          <option :value="7">近 7 天</option>
          <option :value="30">近 30 天</option>
          <option :value="90">近 90 天</option>
        </select>
      </label>
      <button
        type="button"
        class="rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500"
        @click="search"
      >
        刷新
      </button>
      <p class="ml-auto text-xs text-text-muted">共 {{ total }} 位签到用户</p>
    </div>

    <p v-if="error" class="mb-3 rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-2 text-sm text-danger-mc" role="alert">{{ error }}</p>

    <AdminTable :columns="columns" :rows="items" :loading="loading" row-key="user_id" empty-text="暂无签到数据">
      <template #cell-rank="{ row }">
        <span :class="['font-mono font-bold', rankClass(row.rank)]">#{{ row.rank }}</span>
      </template>
      <template #cell-user="{ row }">
        <div class="font-medium text-text-primary">{{ displayName(row) }}</div>
        <div class="text-[11px] text-text-muted">@{{ row.username }}</div>
      </template>
      <template #cell-user_no="{ row }">
        <span class="font-mono text-xs">{{ row.user_no }}</span>
      </template>
      <template #cell-checkin_days="{ row }">
        <span class="font-medium text-grass-300">{{ row.checkin_days }} 天</span>
      </template>
      <template #cell-last_checkin_date="{ row }">
        <span class="text-xs text-text-muted">{{ row.last_checkin_date || '—' }}</span>
      </template>
    </AdminTable>

    <Pagination
      class="mt-4"
      :page="page"
      :page-size="pageSize"
      :total="total"
      :total-pages="totalPages"
      @change="changePage"
    />
  </div>
</template>
