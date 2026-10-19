<script setup lang="ts">
// 工具管理列表
import { onMounted, reactive, ref } from 'vue'
import { RouterLink } from 'vue-router'
import ListShell from '@/components/admin/list-shell.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useUiStore } from '@/stores/ui'
import { listTools, deleteTool } from '@/api/admin-tools'
import type { Tool } from '@/types/tool'
import { ApiError } from '@/types/api'

const ui = useUiStore()
const filters = reactive({ q: '', page: 1, pageSize: 15 })
const items = ref<Tool[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')
const toDelete = ref<Tool | null>(null)
const deleting = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await listTools({ q: filters.q || undefined, page: filters.page, page_size: filters.pageSize })
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
    await deleteTool(toDelete.value.id)
    ui.success('已删除/下架工具')
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
    <ListShell title="工具管理" :loading="loading" :error="error" @retry="load">
      <div class="mb-4 flex flex-wrap items-center gap-3">
        <div class="relative min-w-52 flex-1">
          <input
            v-model="filters.q"
            type="search"
            aria-label="搜索工具"
            placeholder="搜索名称 / 标签"
            class="h-9 w-full rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          />
        </div>
        <RouterLink
          to="/admin/tools/new"
          class="rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500"
        >
          + 新建工具
        </RouterLink>
      </div>

      <div v-if="items.length" class="card overflow-x-auto">
        <table class="w-full min-w-[640px] text-left text-sm">
          <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
            <tr>
              <th scope="col" class="px-4 py-2.5">名称</th>
              <th scope="col" class="px-4 py-2.5">分类</th>
              <th scope="col" class="px-4 py-2.5">状态</th>
              <th scope="col" class="px-4 py-2.5">推荐</th>
              <th scope="col" class="px-4 py-2.5 text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border-dark">
            <tr v-for="item in items" :key="item.id" class="hover:bg-ink-800/60">
              <td class="px-4 py-3">
                <div class="font-medium text-text-primary">{{ item.name }}</div>
                <div class="text-xs text-text-muted">{{ item.slug }}</div>
              </td>
              <td class="px-4 py-3 text-text-secondary">{{ item.category }}</td>
              <td class="px-4 py-3"><StatusBadge :status="item.status" /></td>
              <td class="px-4 py-3">{{ item.is_featured ? '✓' : '—' }}</td>
              <td class="px-4 py-3">
                <div class="flex justify-end gap-1.5 text-xs">
                  <RouterLink
                    :to="`/admin/tools/${item.id}/edit`"
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
      <div v-else-if="!loading" class="card p-10 text-center text-sm text-text-muted">暂无工具。</div>
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
      title="删除工具"
      :message="`确定删除「${toDelete?.name}」吗？`"
      confirm-text="确认删除"
      dangerous
      :confirm-disabled="deleting"
      @confirm="confirmDelete"
      @cancel="toDelete = null"
    />
  </div>
</template>