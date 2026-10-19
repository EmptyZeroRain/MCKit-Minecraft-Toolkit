<script setup lang="ts">
// 百科管理列表
import { onMounted, reactive, ref } from 'vue'
import { RouterLink } from 'vue-router'
import ListShell from '@/components/admin/list-shell.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useUiStore } from '@/stores/ui'
import { listWiki, deleteWiki } from '@/api/admin-wiki'
import type { WikiEntry } from '@/types/wiki'
import { ApiError } from '@/types/api'
import { timeAgo } from '@/utils/format'

const ui = useUiStore()
const filters = reactive<{
  q: string
  status: 'published' | 'draft' | ''
  page: number
  pageSize: number
}>({ q: '', status: '', page: 1, pageSize: 15 })
const items = ref<WikiEntry[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')
const toDelete = ref<WikiEntry | null>(null)
const deleting = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await listWiki({
      q: filters.q || undefined,
      status: filters.status || undefined,
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

async function confirmDelete() {
  if (!toDelete.value) return
  deleting.value = true
  try {
    await deleteWiki(toDelete.value.id)
    ui.success('已删除百科条目')
    toDelete.value = null
    load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '删除失败')
  } finally {
    deleting.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <ListShell title="百科管理" :loading="loading" :error="error" @retry="load">
      <div class="mb-4 flex flex-wrap items-center gap-3">
        <div class="relative min-w-52 flex-1">
          <input
            v-model="filters.q"
            type="search"
            aria-label="搜索百科"
            placeholder="搜索标题 / 标签"
            class="h-9 w-full rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          />
        </div>
        <select
          v-model="filters.status"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
        >
          <option value="">全部状态</option>
          <option value="published">已发布</option>
          <option value="draft">草稿</option>
        </select>
        <RouterLink
          to="/admin/wiki/new"
          class="rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500"
        >
          + 新建百科
        </RouterLink>
      </div>

      <div v-if="items.length" class="card overflow-x-auto">
        <table class="w-full min-w-[640px] text-left text-sm">
          <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
            <tr>
              <th scope="col" class="px-4 py-2.5">标题</th>
              <th scope="col" class="px-4 py-2.5">类型</th>
              <th scope="col" class="px-4 py-2.5">状态</th>
              <th scope="col" class="px-4 py-2.5">更新</th>
              <th scope="col" class="px-4 py-2.5 text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border-dark">
            <tr v-for="item in items" :key="item.id" class="hover:bg-ink-800/60">
              <td class="px-4 py-3">
                <div class="font-medium text-text-primary">{{ item.title }}</div>
                <div class="text-xs text-text-muted">{{ item.slug }}</div>
              </td>
              <td class="px-4 py-3 text-text-secondary">{{ item.content_type }}</td>
              <td class="px-4 py-3"><StatusBadge :status="item.status" /></td>
              <td class="px-4 py-3 text-xs text-text-muted">{{ timeAgo(item.updated_at) }}</td>
              <td class="px-4 py-3">
                <div class="flex justify-end gap-1.5 text-xs">
                  <RouterLink
                    :to="`/admin/wiki/${item.id}/edit`"
                    class="rounded border border-border-light px-2 py-1 text-grass-400 hover:bg-ink-700"
                  >
                    编辑
                  </RouterLink>
                  <button
                    type="button"
                    class="rounded border border-danger-mc/40 px-2 py-1 text-danger-mc hover:bg-danger-mc/10"
                    @click="toDelete = item"
                  >
                    删除
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-else-if="!loading" class="card p-10 text-center text-sm text-text-muted">暂无百科条目。</div>
    </ListShell>

    <Pagination
      :page="filters.page"
      :page-size="filters.pageSize"
      :total="total"
      :total-pages="totalPages"
      @change="onPageChange"
    />

    <ConfirmDialog
      :open="!!toDelete"
      title="删除百科"
      :message="`确定删除「${toDelete?.title}」吗？`"
      confirm-text="确认删除"
      dangerous
      :confirm-disabled="deleting"
      @confirm="confirmDelete"
      @cancel="toDelete = null"
    />
  </div>
</template>