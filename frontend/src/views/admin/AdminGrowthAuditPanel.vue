<script setup lang="ts">
// 成长审计日志（jingyan.md §10.5）。
//
// 所有人工调整、冲正、规则 / 任务 / 成就变更与历史补算均在此留痕，
// 包含操作者、目标用户、动作、原因与请求 ID。
import { onMounted, reactive, ref } from 'vue'
import AdminTable from '@/components/admin/AdminTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import { ApiError } from '@/types/api'
import type { AdminTableColumn } from '@/types/admin'
import { fetchGrowthAudit } from '@/api/growth'
import type { GrowthAuditLog } from '@/types/growth'

const loading = ref(false)
const error = ref('')
const items = ref<GrowthAuditLog[]>([])
const total = ref(0)
const totalPages = ref(1)

const filters = reactive<{ operator_id: string; target_user_id: string; action: string; page: number; pageSize: number }>(
  { operator_id: '', target_user_id: '', action: '', page: 1, pageSize: 20 },
)

const inputClass =
  'w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none'

const columns: AdminTableColumn[] = [
  { key: 'created_at', label: '时间', width: '170px' },
  { key: 'action', label: '动作', width: '150px' },
  { key: 'operator_user_id', label: '操作者', width: '220px' },
  { key: 'target_user_id', label: '目标用户', width: '220px' },
  { key: 'entity', label: '对象' },
  { key: 'reason', label: '原因' },
]

function formatTime(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString('zh-CN')
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await fetchGrowthAudit({
      operator_id: filters.operator_id.trim() || undefined,
      target_user_id: filters.target_user_id.trim() || undefined,
      action: filters.action.trim() || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    })
    items.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '审计加载失败'
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
</script>

<template>
  <div>
    <form class="mb-4 grid gap-3 sm:grid-cols-4" @submit.prevent="search">
      <input v-model="filters.operator_id" :class="inputClass" type="text" placeholder="操作者 ID" />
      <input v-model="filters.target_user_id" :class="inputClass" type="text" placeholder="目标用户 ID" />
      <input v-model="filters.action" :class="inputClass" type="text" placeholder="动作（如 xp.adjust）" />
      <button type="submit" class="rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-secondary transition hover:bg-ink-600">
        查询
      </button>
    </form>

    <p v-if="error" class="mb-3 rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-2 text-sm text-danger-mc" role="alert">{{ error }}</p>

    <AdminTable :columns="columns" :rows="items" :loading="loading" row-key="id" empty-text="暂无审计记录">
      <template #cell-created_at="{ row }">{{ formatTime(row.created_at) }}</template>
      <template #cell-action="{ row }">
        <span class="font-mono text-xs text-text-primary">{{ row.action }}</span>
      </template>
      <template #cell-operator_user_id="{ row }">
        <span class="font-mono text-xs text-text-muted">{{ row.operator_user_id || '—' }}</span>
      </template>
      <template #cell-target_user_id="{ row }">
        <span class="font-mono text-xs text-text-muted">{{ row.target_user_id || '—' }}</span>
      </template>
      <template #cell-entity="{ row }">
        <span class="text-xs text-text-muted">{{ row.entity_type }}{{ row.entity_id ? ` · ${row.entity_id}` : '' }}</span>
      </template>
    </AdminTable>

    <div class="mt-4">
      <Pagination :page="filters.page" :page-size="filters.pageSize" :total="total" :total-pages="totalPages" @change="changePage" />
    </div>
  </div>
</template>
