<script setup lang="ts">
// 系统管理 / 会话管理（需 ops.sessions）。
//
// 列出普通用户与管理员的有效会话：会话 ID 仅以不可逆指纹展示，管理端无法
// 借此冒用登录态；「踢下线」按指纹定位并立即删除目标会话，操作写审计。
// 当前管理员自己的会话禁止在此踢下线，避免误操作导致自锁。
import { onMounted, reactive, ref } from 'vue'
import AdminTable from '@/components/admin/AdminTable.vue'
import ListShell from '@/components/admin/list-shell.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useUiStore } from '@/stores/ui'
import { ApiError } from '@/types/api'
import type { AdminTableColumn } from '@/types/admin'
import {
  listAdminSessions,
  revokeAdminSession,
  type AdminSessionAccountType,
  type AdminSessionItem,
} from '@/api/admin-sessions'
import { formatDateTime } from '@/utils/format'

const ui = useUiStore()

const inputClass =
  'w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none'

const filters = reactive<{
  q: string
  accountType: 'all' | AdminSessionAccountType
  page: number
  pageSize: number
}>({ q: '', accountType: 'all', page: 1, pageSize: 20 })

const items = ref<AdminSessionItem[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')
const busyId = ref('')

const columns: AdminTableColumn[] = [
  { key: 'username', label: '账号' },
  { key: 'account_type', label: '类型', width: '110px' },
  { key: 'last_seen_at', label: '最近活动', width: '170px' },
  { key: 'created_at', label: '登录时间', width: '170px' },
  { key: 'expires_at', label: '过期时间', width: '170px' },
  { key: 'action', label: '操作', align: 'right', width: '110px' },
]

function accountTypeLabel(type: AdminSessionAccountType): string {
  return type === 'user' ? '普通用户' : '管理员'
}

function roleLabel(item: AdminSessionItem): string {
  switch (item.role) {
    case 'admin':
      return '超级管理员'
    case 'editor':
      return '编辑'
    case 'user':
      return '普通用户'
    default:
      return item.role
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await listAdminSessions({
      q: filters.q.trim() || undefined,
      account_type: filters.accountType,
      page: filters.page,
      page_size: filters.pageSize,
    })
    items.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '会话列表加载失败'
  } finally {
    loading.value = false
  }
}

function search() {
  filters.page = 1
  void load()
}

function onPageChange(page: number) {
  filters.page = page
  void load()
}

async function revoke(item: AdminSessionItem) {
  if (item.current) return
  if (
    !window.confirm(
      `确定将「${item.username}」的该会话踢下线？目标设备会立即退出登录，需要重新登录。`,
    )
  )
    return
  busyId.value = item.id
  try {
    await revokeAdminSession(item.id)
    ui.success(`已踢下线：${item.username}`)
    // 当前页最后一条被移除时回退一页，避免停留在空页无法翻回。
    if (items.value.length === 1 && filters.page > 1) filters.page -= 1
    await load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '踢下线失败')
  } finally {
    busyId.value = ''
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <section class="card p-4">
      <div class="mb-3">
        <h2 class="text-sm font-medium text-text-primary">全站会话</h2>
        <p class="mt-0.5 text-xs text-text-muted">
          覆盖普通用户与管理员。列表只展示会话指纹，不暴露可用于冒用登录态的 SessionID；
          踢下线会立即注销目标设备的会话。
        </p>
      </div>
      <form class="flex flex-wrap gap-3" @submit.prevent="search">
        <input
          v-model="filters.q"
          :class="inputClass"
          class="sm:max-w-md"
          type="search"
          aria-label="搜索账号用户名"
          placeholder="搜索账号用户名"
        />
        <select
          v-model="filters.accountType"
          aria-label="账号类型筛选"
          class="h-9 rounded-md border border-border-dark bg-ink-900 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="search"
        >
          <option value="all">全部账号</option>
          <option value="user">普通用户</option>
          <option value="admin">管理员</option>
        </select>
        <button
          type="submit"
          class="rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-secondary transition hover:bg-ink-600"
        >
          查询
        </button>
        <button
          type="button"
          class="rounded-md border border-border-light px-4 py-2 text-sm text-text-secondary transition hover:bg-ink-700"
          @click="load"
        >
          刷新
        </button>
        <span class="self-center text-xs text-text-muted">共 {{ total }} 个有效会话</span>
      </form>
    </section>

    <ListShell title="会话列表" :loading="loading" :error="error" @retry="load">
      <AdminTable
        :columns="columns"
        :rows="items"
        row-key="id"
        empty-text="暂无符合条件的有效会话"
      >
        <template #cell-username="{ row }">
          <div class="min-w-0">
            <p class="flex items-center gap-2 truncate font-medium text-text-primary">
              {{ row.username }}
              <span
                v-if="row.current"
                class="rounded bg-grass-500/15 px-1.5 py-0.5 text-[10px] text-grass-300"
              >
                当前会话
              </span>
            </p>
            <p class="truncate text-xs text-text-muted">
              {{ roleLabel(row) }} · 指纹 <span class="font-mono">{{ row.id }}</span>
            </p>
          </div>
        </template>
        <template #cell-account_type="{ row }">
          <span
            class="rounded-full px-2 py-0.5 text-xs"
            :class="
              row.account_type === 'admin'
                ? 'bg-amber-500/15 text-amber-300'
                : 'bg-ink-700 text-text-secondary'
            "
          >
            {{ accountTypeLabel(row.account_type) }}
          </span>
        </template>
        <template #cell-last_seen_at="{ row }">
          <span class="text-xs text-text-secondary">{{ formatDateTime(row.last_seen_at) }}</span>
        </template>
        <template #cell-created_at="{ row }">
          <span class="text-xs text-text-muted">{{ formatDateTime(row.created_at) }}</span>
        </template>
        <template #cell-expires_at="{ row }">
          <span class="text-xs text-text-muted">{{ formatDateTime(row.expires_at) }}</span>
        </template>
        <template #cell-action="{ row }">
          <button
            type="button"
            class="rounded border px-2 py-1 text-xs transition disabled:cursor-not-allowed disabled:opacity-40"
            :class="
              row.current
                ? 'border-border-dark text-text-muted'
                : 'border-red-500/40 text-red-300 hover:bg-red-500/10'
            "
            :disabled="row.current || busyId === row.id"
            :title="row.current ? '当前会话请在右上角用户菜单退出登录' : '立即注销该会话'"
            @click="revoke(row)"
          >
            {{ row.current ? '当前会话' : busyId === row.id ? '处理中…' : '踢下线' }}
          </button>
        </template>
      </AdminTable>

      <Pagination
        :page="filters.page"
        :page-size="filters.pageSize"
        :total="total"
        :total-pages="totalPages"
        @change="onPageChange"
      />
    </ListShell>
  </div>
</template>
