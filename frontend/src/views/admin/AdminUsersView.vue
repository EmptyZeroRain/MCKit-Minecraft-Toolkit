<script setup lang="ts">
// 普通用户管理（后台）：账号列表 + 个人数据计数。
// 仅 admin 角色可见（侧边栏过滤 + 后端 403 兜底）。
// 本页当前为只读视图；启用/禁用、重置密码、删除、导出 CSV 见后续迭代。
import { onMounted, reactive, ref } from 'vue'
import ListShell from '@/components/admin/list-shell.vue'
import Pagination from '@/components/common/Pagination.vue'
import { listUsers, type UserListQuery } from '@/api/admin-users'
import type { PlatformUserRow } from '@/types/admin'
import { ApiError } from '@/types/api'
import { timeAgo } from '@/utils/format'

const filters = reactive<{ q: string; status: '' | 'active' | 'disabled'; page: number; pageSize: number }>({
  q: '',
  status: '',
  page: 1,
  pageSize: 15,
})

const items = ref<PlatformUserRow[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const params: UserListQuery = {
      q: filters.q || undefined,
      status: filters.status || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    }
    const res = await listUsers(params)
    items.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

function onSearch() {
  filters.page = 1
  load()
}

function onPageChange(p: number) {
  filters.page = p
  load()
}

onMounted(load)
</script>

<template>
  <div class="space-y-5">
    <ListShell title="普通用户" :loading="loading" :error="error" @retry="load">
      <div class="mb-4 flex flex-wrap items-center gap-3">
        <input
          v-model="filters.q"
          type="search"
          aria-label="搜索用户名"
          placeholder="搜索用户名"
          class="h-9 w-full max-w-xs rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          @change="onSearch"
        />
        <select
          v-model="filters.status"
          aria-label="按状态筛选"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="onSearch"
        >
          <option value="">全部状态</option>
          <option value="active">仅启用</option>
          <option value="disabled">仅禁用</option>
        </select>
      </div>

      <div v-if="items.length" class="card overflow-x-auto">
        <table class="w-full min-w-[860px] text-left text-sm">
          <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
            <tr>
              <th scope="col" class="px-4 py-2.5">用户名</th>
              <th scope="col" class="px-4 py-2.5">状态</th>
              <th scope="col" class="px-4 py-2.5">注册</th>
              <th scope="col" class="px-4 py-2.5">最后登录</th>
              <th scope="col" class="px-4 py-2.5 text-right">收藏</th>
              <th scope="col" class="px-4 py-2.5 text-right">最近使用</th>
              <th scope="col" class="px-4 py-2.5 text-right">常用种子</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border-dark">
            <tr v-for="item in items" :key="item.id" class="hover:bg-ink-800/60">
              <td class="px-4 py-3 font-medium text-text-primary">{{ item.username }}</td>
              <td class="px-4 py-3">
                <span
                  class="rounded px-2 py-0.5 text-xs"
                  :class="item.is_active ? 'bg-grass-500/15 text-grass-300' : 'bg-danger-mc/15 text-danger-mc'"
                >
                  {{ item.is_active ? '启用' : '禁用' }}
                </span>
              </td>
              <td class="px-4 py-3 text-xs text-text-muted">{{ timeAgo(item.created_at) }}</td>
              <td class="px-4 py-3 text-xs text-text-muted">
                {{ item.last_login_at ? timeAgo(item.last_login_at) : '从未登录' }}
              </td>
              <td class="px-4 py-3 text-right text-text-secondary">{{ item.favorite_count }}</td>
              <td class="px-4 py-3 text-right text-text-secondary">{{ item.recent_count }}</td>
              <td class="px-4 py-3 text-right text-text-secondary">{{ item.seed_count }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-else-if="!loading" class="card p-10 text-center text-sm text-text-muted">暂无普通用户。</div>
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
