<script setup lang="ts">
// 审计日志（需求 6.4）
import { onMounted, reactive, ref } from 'vue'
import ListShell from '@/components/admin/list-shell.vue'
import Pagination from '@/components/common/Pagination.vue'
import { listAuditLogs } from '@/api/admin-system'
import type { AuditLog } from '@/types/audit'
import { ApiError } from '@/types/api'
import { formatDateTime } from '@/utils/format'

const filters = reactive<{
  action: string
  objectType: string
  result: 'success' | 'failure' | ''
  page: number
  pageSize: number
}>({ action: '', objectType: '', result: '', page: 1, pageSize: 20 })
const items = ref<AuditLog[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await listAuditLogs({
      action: filters.action || undefined,
      object_type: filters.objectType || undefined,
      result: filters.result || undefined,
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

function resultClass(result: string): string {
  return result === 'success' ? 'text-grass-300' : 'text-danger-mc'
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
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
        <select
          v-model="filters.result"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="filters.page = 1; load()"
        >
          <option value="">全部结果</option>
          <option value="success">成功</option>
          <option value="failure">失败</option>
        </select>
      </div>

      <div v-if="items.length" class="card overflow-x-auto">
        <table class="w-full min-w-[760px] text-left text-sm">
          <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
            <tr>
              <th scope="col" class="px-4 py-2.5">时间</th>
              <th scope="col" class="px-4 py-2.5">操作者</th>
              <th scope="col" class="px-4 py-2.5">动作</th>
              <th scope="col" class="px-4 py-2.5">对象</th>
              <th scope="col" class="px-4 py-2.5">结果</th>
              <th scope="col" class="px-4 py-2.5">请求 ID</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border-dark">
            <tr v-for="item in items" :key="item.id" class="hover:bg-ink-800/60">
              <td class="whitespace-nowrap px-4 py-3 text-xs text-text-secondary">{{ formatDateTime(item.created_at) }}</td>
              <td class="px-4 py-3 text-text-secondary">{{ item.actor_username || item.actor_admin_id || '—' }}</td>
              <td class="px-4 py-3 font-mono text-xs text-grass-300">{{ item.action }}</td>
              <td class="px-4 py-3">
                <span class="text-text-primary">{{ item.object_type }}</span>
                <span v-if="item.object_id" class="ml-1 font-mono text-xs text-text-muted">{{ item.object_id.slice(0, 8) }}</span>
              </td>
              <td class="px-4 py-3 text-xs font-medium" :class="resultClass(item.result)">{{ item.result }}</td>
              <td class="px-4 py-3 font-mono text-xs text-text-muted">{{ item.request_id || '—' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-else-if="!loading" class="card p-10 text-center text-sm text-text-muted">暂无日志。</div>
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